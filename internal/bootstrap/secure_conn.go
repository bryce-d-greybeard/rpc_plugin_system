package bootstrap

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

const (
	maxSecureFrameSize   = 16 << 20
	securePacketData     = 0
	securePacketRekey    = 1
	secureRekeyPacketLen = 9
)

type secureConn struct {
	net.Conn
	reader *secureReader
	writer *secureWriter
}

type secureTransportConfig struct {
	writeKey     []byte
	readKey      []byte
	connectionID uint64
	pluginID     string
	sessionID    string
	writeLabel   string
	readLabel    string
	generationID uint64
	policy       RekeyPolicy
}

func NewSecureConn(conn net.Conn, writeKey, readKey []byte) (net.Conn, error) {
	if conn == nil {
		return nil, fmt.Errorf("conn is required")
	}
	policy := DefaultRekeyPolicy()
	startedAt := time.Now().UTC()
	writer, err := newSecureWriter(conn, writeKey, nil, policy, startedAt)
	if err != nil {
		return nil, err
	}
	reader, err := newSecureReader(conn, readKey, nil, policy, startedAt)
	if err != nil {
		return nil, err
	}
	return &secureConn{Conn: conn, reader: reader, writer: writer}, nil
}

func (c *secureConn) Read(p []byte) (int, error)  { return c.reader.Read(p) }
func (c *secureConn) Write(p []byte) (int, error) { return c.writer.Write(p) }

type secureReader struct {
	r         io.Reader
	aead      cipher.AEAD
	policy    RekeyPolicy
	startedAt time.Time
	mu        sync.Mutex
	seq       uint64
	bytesRead uint64
	buf       []byte
	cfg       secureTransportConfig
	aad       []byte
}

type secureWriter struct {
	w            io.Writer
	aead         cipher.AEAD
	policy       RekeyPolicy
	startedAt    time.Time
	mu           sync.Mutex
	seq          uint64
	bytesWritten uint64
	cfg          secureTransportConfig
	aad          []byte
}

func newSecureReader(r io.Reader, key []byte, aad []byte, policy RekeyPolicy, startedAt time.Time) (*secureReader, error) {
	aead, err := newAEAD(key)
	if err != nil {
		return nil, err
	}
	return &secureReader{r: r, aead: aead, aad: append([]byte(nil), aad...), policy: policy, startedAt: startedAt}, nil
}

func newSecureWriter(w io.Writer, key []byte, aad []byte, policy RekeyPolicy, startedAt time.Time) (*secureWriter, error) {
	aead, err := newAEAD(key)
	if err != nil {
		return nil, err
	}
	return &secureWriter{w: w, aead: aead, aad: append([]byte(nil), aad...), policy: policy, startedAt: startedAt}, nil
}

func newConfiguredSecureReader(r io.Reader, cfg secureTransportConfig) (*secureReader, error) {
	reader, err := newSecureReader(r, cfg.readKey, secureAAD(cfg.pluginID, cfg.generationID, cfg.sessionID, cfg.connectionID, cfg.readLabel), cfg.policy, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	reader.cfg = cloneSecureTransportConfig(cfg)
	return reader, nil
}

func newConfiguredSecureWriter(w io.Writer, cfg secureTransportConfig) (*secureWriter, error) {
	writer, err := newSecureWriter(w, cfg.writeKey, secureAAD(cfg.pluginID, cfg.generationID, cfg.sessionID, cfg.connectionID, cfg.writeLabel), cfg.policy, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	writer.cfg = cloneSecureTransportConfig(cfg)
	return writer, nil
}

func cloneSecureTransportConfig(cfg secureTransportConfig) secureTransportConfig {
	cfg.writeKey = append([]byte(nil), cfg.writeKey...)
	cfg.readKey = append([]byte(nil), cfg.readKey...)
	return cfg
}

func newAEAD(key []byte) (cipher.AEAD, error) {
	if len(key) != RootKeySize {
		return nil, fmt.Errorf("invalid secure transport key size: %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("new aes cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("new gcm: %w", err)
	}
	return aead, nil
}

func (r *secureReader) Read(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.buf) == 0 {
		if err := r.fill(); err != nil {
			return 0, err
		}
	}
	n := copy(p, r.buf)
	r.buf = r.buf[n:]
	return n, nil
}

func (r *secureReader) fill() error {
	for {
		var lenBuf [4]byte
		if _, err := io.ReadFull(r.r, lenBuf[:]); err != nil {
			return err
		}
		n := binary.BigEndian.Uint32(lenBuf[:])
		if n == 0 || n > maxSecureFrameSize {
			return fmt.Errorf("invalid secure frame size: %d", n)
		}
		ciphertext := make([]byte, n)
		if _, err := io.ReadFull(r.r, ciphertext); err != nil {
			return err
		}
		plain, err := r.aead.Open(nil, nonceForSeq(r.seq), ciphertext, r.aad)
		if err != nil {
			return fmt.Errorf("open secure frame: %w", err)
		}
		r.seq++
		r.bytesRead += uint64(len(plain))
		if len(plain) == 0 {
			return fmt.Errorf("invalid secure packet size: 0")
		}
		switch plain[0] {
		case securePacketData:
			if r.policy.MaxConnectionAge > 0 && !r.startedAt.IsZero() && time.Since(r.startedAt) >= r.policy.MaxConnectionAge {
				return fmt.Errorf("secure transport rekey required: age limit")
			}
			r.buf = append(r.buf[:0], plain[1:]...)
			return nil
		case securePacketRekey:
			if err := r.applyRekeyPacket(plain); err != nil {
				return err
			}
		default:
			return fmt.Errorf("invalid secure packet type: %d", plain[0])
		}
	}
}

func (w *secureWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	packet := encodeDataPacket(p)
	if err := w.ensureWritableForPacket(len(packet)); err != nil {
		return 0, err
	}
	if err := w.writePacket(packet); err != nil {
		return 0, err
	}
	return len(p), nil
}

func encodeDataPacket(p []byte) []byte {
	packet := make([]byte, len(p)+1)
	packet[0] = securePacketData
	copy(packet[1:], p)
	return packet
}

func nonceForSeq(seq uint64) []byte {
	nonce := make([]byte, 12)
	binary.BigEndian.PutUint64(nonce[4:], seq)
	return nonce
}

func NewLabeledSecureConn(conn net.Conn, writeKey, readKey []byte, connectionID uint64, pluginID string, generationID uint64, sessionID, writeLabel, readLabel string) (net.Conn, error) {
	return NewLabeledSecureConnWithPolicy(conn, writeKey, readKey, connectionID, pluginID, generationID, sessionID, writeLabel, readLabel, DefaultRekeyPolicy())
}

func NewLabeledSecureConnWithPolicy(conn net.Conn, writeKey, readKey []byte, connectionID uint64, pluginID string, generationID uint64, sessionID, writeLabel, readLabel string, policy RekeyPolicy) (net.Conn, error) {
	if conn == nil {
		return nil, fmt.Errorf("conn is required")
	}
	derivedWriteKey, err := DeriveTransportKey(writeKey, connectionID, writeLabel)
	if err != nil {
		return nil, err
	}
	derivedReadKey, err := DeriveTransportKey(readKey, connectionID, readLabel)
	if err != nil {
		return nil, err
	}
	cfg := secureTransportConfig{
		writeKey:     derivedWriteKey,
		readKey:      derivedReadKey,
		connectionID: connectionID,
		pluginID:     pluginID,
		sessionID:    sessionID,
		writeLabel:   writeLabel,
		readLabel:    readLabel,
		generationID: generationID,
		policy:       policy,
	}
	reader, err := newConfiguredSecureReader(conn, cfg)
	if err != nil {
		return nil, err
	}
	writer, err := newConfiguredSecureWriter(conn, cfg)
	if err != nil {
		return nil, err
	}
	return &secureConn{Conn: conn, reader: reader, writer: writer}, nil
}

func secureAAD(pluginID string, generationID uint64, sessionID string, connectionID uint64, direction string) []byte {
	return []byte(fmt.Sprintf("rpc_plugin_system/secure/v1/%s/%d/%s/%d/%s", pluginID, generationID, sessionID, connectionID, direction))
}

func (r *secureReader) applyRekeyPacket(packet []byte) error {
	if len(packet) != 9 {
		return fmt.Errorf("invalid secure rekey packet size: %d", len(packet))
	}
	nextGeneration := binary.BigEndian.Uint64(packet[1:])
	if nextGeneration != r.cfg.generationID+1 {
		return fmt.Errorf("invalid secure rekey generation: got %d want %d", nextGeneration, r.cfg.generationID+1)
	}
	nextKey, err := DeriveTransportRekeyKey(r.cfg.readKey, r.cfg.readLabel, nextGeneration)
	if err != nil {
		return err
	}
	nextAEAD, err := newAEAD(nextKey)
	if err != nil {
		zeroBytes(nextKey)
		return err
	}
	zeroBytes(r.cfg.readKey)
	r.cfg.readKey = nextKey
	r.cfg.generationID = nextGeneration
	r.aead = nextAEAD
	r.aad = secureAAD(r.cfg.pluginID, nextGeneration, r.cfg.sessionID, r.cfg.connectionID, r.cfg.readLabel)
	r.startedAt = time.Now().UTC()
	r.seq = 0
	r.bytesRead = 0
	return nil
}

func (w *secureWriter) ensureWritableForPacket(packetLen int) error {
	if len(w.cfg.writeKey) == 0 {
		return checkRekeyBoundary(w.policy, w.startedAt, w.seq, w.bytesWritten, uint64(packetLen))
	}
	for requiresRekeyBeforeData(w.policy, w.startedAt, w.seq, w.bytesWritten, uint64(packetLen)) {
		if err := w.writeRekeyPacket(); err != nil {
			return err
		}
	}
	return nil
}

func requiresRekeyBeforeData(policy RekeyPolicy, startedAt time.Time, seq uint64, totalBytes uint64, nextBytes uint64) bool {
	if reason := rekeyBoundaryReason(policy, startedAt, seq, totalBytes, nextBytes); reason != "" {
		return true
	}
	if policy.MaxFramesPerDirection > 0 && policy.MaxFramesPerDirection <= seq+1 {
		return true
	}
	if policy.MaxBytesPerDirection > 0 {
		reserve := uint64(secureRekeyPacketLen) + nextBytes
		if reserve > policy.MaxBytesPerDirection || totalBytes > policy.MaxBytesPerDirection-reserve {
			return true
		}
	}
	return false
}

func rekeyBoundaryReason(policy RekeyPolicy, startedAt time.Time, seq uint64, totalBytes uint64, nextBytes uint64) string {
	now := time.Now().UTC()
	if policy.MaxConnectionAge > 0 && !startedAt.IsZero() && now.Sub(startedAt) >= policy.MaxConnectionAge {
		return "age limit"
	}
	if policy.MaxFramesPerDirection > 0 && seq >= policy.MaxFramesPerDirection {
		return "frame limit"
	}
	if policy.MaxBytesPerDirection > 0 && nextBytes > 0 && totalBytes > policy.MaxBytesPerDirection-nextBytes {
		return "byte limit"
	}
	return ""
}

func (w *secureWriter) writeRekeyPacket() error {
	if err := checkRekeyBoundary(w.policy, w.startedAt, w.seq, w.bytesWritten, secureRekeyPacketLen); err != nil {
		return err
	}
	nextGeneration := w.cfg.generationID + 1
	packet := make([]byte, secureRekeyPacketLen)
	packet[0] = securePacketRekey
	binary.BigEndian.PutUint64(packet[1:], nextGeneration)
	if err := w.writePacket(packet); err != nil {
		return err
	}
	nextKey, err := DeriveTransportRekeyKey(w.cfg.writeKey, w.cfg.writeLabel, nextGeneration)
	if err != nil {
		return err
	}
	nextAEAD, err := newAEAD(nextKey)
	if err != nil {
		zeroBytes(nextKey)
		return err
	}
	zeroBytes(w.cfg.writeKey)
	w.cfg.writeKey = nextKey
	w.cfg.generationID = nextGeneration
	w.aead = nextAEAD
	w.aad = secureAAD(w.cfg.pluginID, nextGeneration, w.cfg.sessionID, w.cfg.connectionID, w.cfg.writeLabel)
	w.startedAt = time.Now().UTC()
	w.seq = 0
	w.bytesWritten = 0
	return nil
}

func (w *secureWriter) writePacket(packet []byte) error {
	sealed := w.aead.Seal(nil, nonceForSeq(w.seq), packet, w.aad)
	if len(sealed) > maxSecureFrameSize {
		return fmt.Errorf("secure frame too large: %d", len(sealed))
	}
	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(sealed)))
	if _, err := w.w.Write(lenBuf[:]); err != nil {
		return err
	}
	if _, err := w.w.Write(sealed); err != nil {
		return err
	}
	w.seq++
	w.bytesWritten += uint64(len(packet))
	return nil
}

func (r *secureReader) checkRekeyBoundary(nextBytes uint64) error {
	return checkRekeyBoundary(r.policy, r.startedAt, r.seq, r.bytesRead, nextBytes)
}

func (w *secureWriter) checkRekeyBoundary(nextBytes uint64) error {
	return checkRekeyBoundary(w.policy, w.startedAt, w.seq, w.bytesWritten, nextBytes)
}

func checkRekeyBoundary(policy RekeyPolicy, startedAt time.Time, seq uint64, totalBytes uint64, nextBytes uint64) error {
	if reason := rekeyBoundaryReason(policy, startedAt, seq, totalBytes, nextBytes); reason != "" {
		return fmt.Errorf("secure transport rekey required: %s", reason)
	}
	return nil
}
