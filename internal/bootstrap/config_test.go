package bootstrap

import (
	"os"
	"testing"
	"time"
)

func TestLoadEnvConfig(t *testing.T) {
	t.Setenv(EnvSessionID, "s1")
	t.Setenv(EnvSubstratePublicKey, "pub")
	cfg := LoadEnvConfig()
	if cfg.SessionID != "s1" {
		t.Fatalf("SessionID = %q", cfg.SessionID)
	}
	if string(cfg.SubstratePublicKey) != "pub" {
		t.Fatalf("SubstratePublicKey = %q", string(cfg.SubstratePublicKey))
	}
}

func TestRecordExpired(t *testing.T) {
	r := Record{ExpiresAt: time.Unix(10, 0).UTC()}
	if !r.Expired(time.Unix(11, 0).UTC()) {
		t.Fatal("record should be expired")
	}
	if r.Expired(time.Unix(9, 0).UTC()) {
		t.Fatal("record should not be expired")
	}
	_ = os.Getenv("IGNORE")
}
