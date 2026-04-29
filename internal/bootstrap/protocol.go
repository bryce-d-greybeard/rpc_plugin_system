package bootstrap

import "time"

const ProtocolVersion = 1

type Record struct {
	Version            int       `json:"version"`
	PluginID           string    `json:"plugin_id"`
	SessionID          string    `json:"session_id"`
	Token              string    `json:"token"`
	IssuedAt           time.Time `json:"issued_at"`
	ExpiresAt          time.Time `json:"expires_at"`
	SubstratePublicKey []byte    `json:"substrate_public_key,omitempty"`
}

type Response struct {
	Version         int    `json:"version"`
	PluginID        string `json:"plugin_id"`
	SessionID       string `json:"session_id"`
	Token           string `json:"token"`
	PluginPublicKey []byte `json:"plugin_public_key,omitempty"`
}
