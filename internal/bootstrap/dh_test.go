package bootstrap

import "testing"

func TestDeriveSharedSecretMatches(t *testing.T) {
	a, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair a: %v", err)
	}
	b, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair b: %v", err)
	}
	sa, err := DeriveSharedSecret(a.Private, b.Public)
	if err != nil {
		t.Fatalf("DeriveSharedSecret a: %v", err)
	}
	sb, err := DeriveSharedSecret(b.Private, a.Public)
	if err != nil {
		t.Fatalf("DeriveSharedSecret b: %v", err)
	}
	if string(sa) != string(sb) {
		t.Fatal("shared secrets differ")
	}
}
