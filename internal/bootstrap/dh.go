package bootstrap

import (
	"crypto/ecdh"
	"crypto/rand"
	"fmt"
)

type KeyPair struct {
	Private *ecdh.PrivateKey
	Public  []byte
}

func GenerateKeyPair() (*KeyPair, error) {
	curve := ecdh.X25519()
	priv, err := curve.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate x25519 keypair: %w", err)
	}
	return &KeyPair{Private: priv, Public: priv.PublicKey().Bytes()}, nil
}

func DeriveSharedSecret(priv *ecdh.PrivateKey, peerPublic []byte) ([]byte, error) {
	if priv == nil {
		return nil, fmt.Errorf("private key is required")
	}
	pub, err := ecdh.X25519().NewPublicKey(peerPublic)
	if err != nil {
		return nil, fmt.Errorf("parse peer public key: %w", err)
	}
	secret, err := priv.ECDH(pub)
	if err != nil {
		return nil, fmt.Errorf("derive shared secret: %w", err)
	}
	return secret, nil
}
