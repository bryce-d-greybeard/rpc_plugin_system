package testroot

import (
	"os"
	"path/filepath"
	"testing"
)

// SourceRoot returns the Go module/source root for tests, independent of the
// package directory the test binary starts in.
func SourceRoot(t testing.TB) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		} else if !os.IsNotExist(err) {
			t.Fatalf("stat go.mod in %s: %v", dir, err)
		}
		next := filepath.Dir(dir)
		if next == dir {
			t.Fatalf("could not find source root containing go.mod from %s", dir)
		}
		dir = next
	}
}
