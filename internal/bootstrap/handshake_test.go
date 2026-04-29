//go:build !windows

package bootstrap

import (
	"testing"
	"time"

	"rpc_plugin_system/internal/auth"
)

func TestExchangeRecordAndResponse(t *testing.T) {
	tr, err := NewFIFOTransport(t.TempDir(), "echo", "session-handshake")
	if err != nil {
		t.Fatalf("NewFIFOTransport: %v", err)
	}
	defer tr.Cleanup()

	record := Record{Version: ProtocolVersion, PluginID: "echo", SessionID: "session-handshake", Token: auth.Encode([]byte("tok"))}

	done := make(chan error, 1)
	go func() {
		r, err := tr.OpenRequestReader()
		if err != nil {
			done <- err
			return
		}
		defer r.Close()
		rec, err := ReadRecord(r)
		if err != nil {
			done <- err
			return
		}
		if rec.Token != auth.Encode([]byte("tok")) {
			done <- err
			return
		}
		w, err := tr.OpenResponseWriter()
		if err != nil {
			done <- err
			return
		}
		defer w.Close()
		kp, err := GenerateKeyPair()
		if err != nil {
			done <- err
			return
		}
		done <- WriteResponse(w, Response{Version: ProtocolVersion, PluginID: "echo", SessionID: "session-handshake", Token: auth.Encode([]byte("tok")), PluginPublicKey: kp.Public})
	}()

	resp, err := ExchangeRecordAndResponse(tr, record)
	if err != nil {
		t.Fatalf("ExchangeRecordAndResponse: %v", err)
	}
	if resp.SessionID != "session-handshake" {
		t.Fatalf("SessionID = %q", resp.SessionID)
	}
	if err := <-done; err != nil {
		t.Fatalf("peer: %v", err)
	}
}

func TestValidateResponse(t *testing.T) {
	substrateKP, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair substrate: %v", err)
	}
	pluginKP, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair plugin: %v", err)
	}
	s := &Session{PluginID: "echo", SessionID: "s1", Token: []byte("tok"), ExpiresAt: time.Now().Add(time.Minute), SubstrateKeyPair: substrateKP}
	if err := ValidateResponse(s, Response{Version: ProtocolVersion, PluginID: "echo", SessionID: "s1", Token: auth.Encode([]byte("tok")), PluginPublicKey: pluginKP.Public}, time.Now()); err != nil {
		t.Fatalf("ValidateResponse: %v", err)
	}
	if len(s.SessionRootKey) == 0 {
		t.Fatal("SessionRootKey missing")
	}
}
