package bootstrap

import (
	"testing"
	"time"
)

func TestDefaultRekeyPolicy(t *testing.T) {
	policy := DefaultRekeyPolicy()
	if policy.MaxFramesPerDirection != 1<<32 {
		t.Fatalf("MaxFramesPerDirection = %d", policy.MaxFramesPerDirection)
	}
	if policy.MaxBytesPerDirection != 32<<30 {
		t.Fatalf("MaxBytesPerDirection = %d", policy.MaxBytesPerDirection)
	}
	if policy.MaxConnectionAge != 60*time.Minute {
		t.Fatalf("MaxConnectionAge = %s", policy.MaxConnectionAge)
	}
}

func TestRekeyAdvancesGeneration(t *testing.T) {
	root := make([]byte, RootKeySize)
	for i := range root {
		root[i] = byte(i)
	}
	keys, err := NewKeys(root)
	if err != nil {
		t.Fatalf("NewKeys: %v", err)
	}
	next, err := Rekey(keys)
	if err != nil {
		t.Fatalf("Rekey: %v", err)
	}
	if next.Generation != 2 {
		t.Fatalf("Generation = %d, want 2", next.Generation)
	}
	if string(next.SendKey) == string(keys.SendKey) {
		t.Fatal("send key did not change")
	}
}
