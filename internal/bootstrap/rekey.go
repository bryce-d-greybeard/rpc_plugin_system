package bootstrap

import (
	"crypto/hkdf"
	"crypto/sha256"
	"fmt"
		"strconv"
)

type Keys struct {
	Generation uint64
	RootKey    []byte
	SendKey    []byte
	RecvKey    []byte
}

func NewKeys(rootKey []byte) (*Keys, error) {
	return DeriveGeneration(rootKey, 1)
}

func DeriveGeneration(rootKey []byte, generation uint64) (*Keys, error) {
	if len(rootKey) != RootKeySize {
		return nil, fmt.Errorf("invalid root key size: %d", len(rootKey))
	}
	sendKey, err := deriveLabeled(rootKey, "send", generation)
	if err != nil {
		return nil, err
	}
	recvKey, err := deriveLabeled(rootKey, "recv", generation)
	if err != nil {
		return nil, err
	}
	return &Keys{Generation: generation, RootKey: append([]byte(nil), rootKey...), SendKey: sendKey, RecvKey: recvKey}, nil
}

func Rekey(keys *Keys) (*Keys, error) {
	if keys == nil {
		return nil, fmt.Errorf("keys are required")
	}
	return DeriveGeneration(keys.RootKey, keys.Generation+1)
}

func deriveLabeled(rootKey []byte, label string, generation uint64) ([]byte, error) {
	info := "rpc_plugin_system/rekey/" + label + "/" + strconv.FormatUint(generation, 10)
	out, err := hkdf.Key(sha256.New, rootKey, nil, info, RootKeySize)
	if err != nil {
		return nil, fmt.Errorf("derive %s key: %w", label, err)
	}
	return out, nil
}
