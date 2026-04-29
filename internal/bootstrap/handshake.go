package bootstrap

import (
	"fmt"
	"time"

	"rpc_plugin_system/internal/auth"
)

func ExchangeRecordAndResponse(t Transport, record Record) (Response, error) {
	responseReady := make(chan struct {
		resp Response
		err  error
	}, 1)
	go func() {
		r, err := t.OpenResponseReader()
		if err != nil {
			responseReady <- struct {
				resp Response
				err  error
			}{err: err}
			return
		}
		defer r.Close()
		resp, err := ReadResponse(r)
		responseReady <- struct {
			resp Response
			err  error
		}{resp: resp, err: err}
	}()

	w, err := t.OpenRequestWriter()
	if err != nil {
		return Response{}, fmt.Errorf("open request writer: %w", err)
	}
	if err := WriteRecord(w, record); err != nil {
		_ = w.Close()
		return Response{}, fmt.Errorf("write bootstrap record: %w", err)
	}
	if err := w.Close(); err != nil {
		return Response{}, fmt.Errorf("close request writer: %w", err)
	}

	result := <-responseReady
	if result.err != nil {
		return Response{}, fmt.Errorf("read bootstrap response: %w", result.err)
	}
	return result.resp, nil
}

func ValidateResponse(session *Session, response Response, now time.Time) error {
	if session == nil {
		return fmt.Errorf("session is required")
	}
	if response.Version != ProtocolVersion {
		return fmt.Errorf("protocol version mismatch: got %d want %d", response.Version, ProtocolVersion)
	}
	if response.PluginID != session.PluginID {
		return fmt.Errorf("plugin id mismatch: got %q want %q", response.PluginID, session.PluginID)
	}
	if response.SessionID != session.SessionID {
		return fmt.Errorf("session id mismatch: got %q want %q", response.SessionID, session.SessionID)
	}
	decoded, err := auth.Decode(response.Token)
	if err != nil {
		return fmt.Errorf("decode token: %w", err)
	}
	if !auth.EqualToken(decoded, session.Token) {
		return fmt.Errorf("token mismatch")
	}
	if len(response.PluginPublicKey) == 0 {
		return fmt.Errorf("plugin public key missing")
	}
	if session.SubstrateKeyPair == nil || session.SubstrateKeyPair.Private == nil {
		return fmt.Errorf("substrate keypair missing")
	}
	sharedSecret, err := DeriveSharedSecret(session.SubstrateKeyPair.Private, response.PluginPublicKey)
	if err != nil {
		return fmt.Errorf("derive shared secret: %w", err)
	}
	rootKey, err := DeriveRootKey(sharedSecret, session.Token, session.PluginID, session.SessionID)
	if err != nil {
		return fmt.Errorf("derive root key: %w", err)
	}
	keys, err := NewKeys(rootKey)
	if err != nil {
		return fmt.Errorf("derive initial session keys: %w", err)
	}
	keys, err = Rekey(keys)
	if err != nil {
		return fmt.Errorf("refresh session keys: %w", err)
	}
	session.PluginPublicKey = append([]byte(nil), response.PluginPublicKey...)
	session.SharedSecret = sharedSecret
	session.SessionRootKey = rootKey
	session.SessionKeys = keys
	return nil
}
