package bootstrap

import "testing"

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
