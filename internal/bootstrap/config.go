package bootstrap

import (
	"os"
	"time"
)

const (
	EnvSessionID          = "RPC_PLUGIN_SYSTEM_BOOTSTRAP_SESSION_ID"
	EnvSubstratePublicKey = "RPC_PLUGIN_SYSTEM_BOOTSTRAP_SUBSTRATE_PUBLIC_KEY"
)

type EnvConfig struct {
	SessionID          string
	SubstratePublicKey []byte
}

func LoadEnvConfig() EnvConfig {
	return EnvConfig{
		SessionID:          os.Getenv(EnvSessionID),
		SubstratePublicKey: []byte(os.Getenv(EnvSubstratePublicKey)),
	}
}

func NewRecordFromSession(s *Session, substratePublicKey []byte) Record {
	return Record{
		Version:            ProtocolVersion,
		PluginID:           s.PluginID,
		SessionID:          s.SessionID,
		Token:              string(s.Token),
		IssuedAt:           s.IssuedAt,
		ExpiresAt:          s.ExpiresAt,
		SubstratePublicKey: append([]byte(nil), substratePublicKey...),
	}
}

func NewResponse(pluginID, sessionID string, token string, pluginPublicKey []byte) Response {
	return Response{
		Version:         ProtocolVersion,
		PluginID:        pluginID,
		SessionID:       sessionID,
		Token:           token,
		PluginPublicKey: append([]byte(nil), pluginPublicKey...),
	}
}

func (r Record) Expired(now time.Time) bool {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return now.After(r.ExpiresAt)
}
