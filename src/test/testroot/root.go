package testroot

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

var (
	sourceRootGetwd  = os.Getwd
	sourceRootStat   = os.Stat
	sourceRootFatalf = testing.TB.Fatalf
)

// SourceRoot returns the Go module/source root for tests, independent of the
// package directory the test binary starts in.
func SourceRoot(t testing.TB) string {
	t.Helper()

	dir, err := sourceRoot(sourceRootGetwd, sourceRootStat)
	if err != nil {
		sourceRootFatalf(t, "%v", err)
	}
	return dir
}

func sourceRoot(getwd func() (string, error), stat func(string) (os.FileInfo, error)) (string, error) {
	dir, err := getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}
	for {
		if _, err := stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		} else if !os.IsNotExist(err) {
			return "", fmt.Errorf("stat go.mod in %s: %w", dir, err)
		}
		next := filepath.Dir(dir)
		if next == dir {
			return "", fmt.Errorf("could not find source root containing go.mod from %s", dir)
		}
		dir = next
	}
}
