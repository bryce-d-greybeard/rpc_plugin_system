package bootstrap

import (
	"crypto/hkdf"
	"crypto/sha256"
	"fmt"
	)

const RootKeySize = 32

func DeriveRootKey(sharedSecret, token []byte, pluginID, sessionID string) ([]byte, error) {
	if len(sharedSecret) == 0 {
		return nil, fmt.Errorf("shared secret is required")
	}
	if len(token) == 0 {
		return nil, fmt.Errorf("token is required")
	}
	info := "rpc_plugin_system/bootstrap/v1/" + pluginID + "/" + sessionID
	out, err := hkdf.Key(sha256.New, sharedSecret, token, info, RootKeySize)
	if err != nil {
		return nil, fmt.Errorf("derive root key: %w", err)
	}
	return out, nil
}
