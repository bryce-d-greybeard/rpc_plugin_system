package bootstrap

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
)

const maxSecureFrameSize = 16 << 20

type secureConn struct {
	net.Conn
	reader *secureReader
	writer *secureWriter
}

func NewSecureConn(conn net.Conn, writeKey, readKey []byte) (net.Conn, error) {
	if conn == nil {
		return nil, fmt.Errorf("conn is required")
	}
	writer, err := newSecureWriter(conn, writeKey)
	if err != nil {
		return nil, err
	}
	reader, err := newSecureReader(conn, readKey)
	if err != nil {
		return nil, err
	}
	return &secureConn{Conn: conn, reader: reader, writer: writer}, nil
}

func (c *secureConn) Read(p []byte) (int, error)  { return c.reader.Read(p) }
func (c *secureConn) Write(p []byte) (int, error) { return c.writer.Write(p) }

type secureReader struct {
	r    io.Reader
	aead cipher.AEAD
	mu   sync.Mutex
	seq  uint64
	buf  []byte
}

type secureWriter struct {
	w    io.Writer
	aead cipher.AEAD
	mu   sync.Mutex
	seq  uint64
}

func newSecureReader(r io.Reader, key []byte) (*secureReader, error) {
	aead, err := newAEAD(key)
	if err != nil {
		return nil, err
	}
	return &secureReader{r: r, aead: aead}, nil
}

func newSecureWriter(w io.Writer, key []byte) (*secureWriter, error) {
	aead, err := newAEAD(key)
	if err != nil {
		return nil, err
	}
	return &secureWriter{w: w, aead: aead}, nil
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
	plain, err := r.aead.Open(nil, nonceForSeq(r.seq), ciphertext, nil)
	if err != nil {
		return fmt.Errorf("open secure frame: %w", err)
	}
	r.seq++
	r.buf = plain
	return nil
}

func (w *secureWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	sealed := w.aead.Seal(nil, nonceForSeq(w.seq), p, nil)
	w.seq++
	if len(sealed) > maxSecureFrameSize {
		return 0, fmt.Errorf("secure frame too large: %d", len(sealed))
	}
	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(sealed)))
	if _, err := w.w.Write(lenBuf[:]); err != nil {
		return 0, err
	}
	if _, err := w.w.Write(sealed); err != nil {
		return 0, err
	}
	return len(p), nil
}

func nonceForSeq(seq uint64) []byte {
	nonce := make([]byte, 12)
	binary.BigEndian.PutUint64(nonce[4:], seq)
	return nonce
}

func NewLabeledSecureConn(conn net.Conn, writeKey, readKey []byte, connectionID uint64, writeLabel, readLabel string) (net.Conn, error) {
	derivedWriteKey, err := DeriveTransportKey(writeKey, connectionID, writeLabel)
	if err != nil {
		return nil, err
	}
	derivedReadKey, err := DeriveTransportKey(readKey, connectionID, readLabel)
	if err != nil {
		return nil, err
	}
	return NewSecureConn(conn, derivedWriteKey, derivedReadKey)
}
