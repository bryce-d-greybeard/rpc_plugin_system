package testroot

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSourceRootReportsLookupFailureThroughTestingFatal(t *testing.T) {
	restore := replaceSourceRootHooks(
		func() (string, error) { return "", errors.New("cwd unavailable") },
		nil,
		func(_ testing.TB, format string, args ...any) { panic(fmt.Sprintf(format, args...)) },
	)
	defer restore()

	message := panicMessage(t, func() { SourceRoot(t) })
	if !strings.Contains(message, "get working directory: cwd unavailable") {
		t.Fatalf("SourceRoot fatal message = %q, want cwd failure", message)
	}
}

func TestSourceRootAscendsToModuleRoot(t *testing.T) {
	moduleRoot := filepath.Join(t.TempDir(), "src")
	start := filepath.Join(moduleRoot, "test", "testroot")

	got, err := sourceRoot(
		func() (string, error) { return start, nil },
		func(path string) (os.FileInfo, error) {
			if path == filepath.Join(moduleRoot, "go.mod") {
				return nil, nil
			}
			return nil, fs.ErrNotExist
		},
	)
	if err != nil {
		t.Fatalf("sourceRoot returned error: %v", err)
	}
	if got != moduleRoot {
		t.Fatalf("sourceRoot = %q, want %q", got, moduleRoot)
	}
}

func TestSourceRootReturnsGetwdError(t *testing.T) {
	_, err := sourceRoot(func() (string, error) { return "", errors.New("boom") }, nil)
	if err == nil || !strings.Contains(err.Error(), "get working directory: boom") {
		t.Fatalf("sourceRoot getwd error = %v, want wrapped cwd error", err)
	}
}

func TestSourceRootReturnsStatError(t *testing.T) {
	start := t.TempDir()
	_, err := sourceRoot(
		func() (string, error) { return start, nil },
		func(string) (os.FileInfo, error) { return nil, errors.New("permission denied") },
	)
	if err == nil || !strings.Contains(err.Error(), "stat go.mod in "+start+": permission denied") {
		t.Fatalf("sourceRoot stat error = %v, want wrapped stat error", err)
	}
}

func TestSourceRootReturnsMissingModuleRootError(t *testing.T) {
	start := filepath.Join(t.TempDir(), "nested")
	_, err := sourceRoot(
		func() (string, error) { return start, nil },
		func(string) (os.FileInfo, error) { return nil, fs.ErrNotExist },
	)
	if err == nil || !strings.Contains(err.Error(), "could not find source root containing go.mod from ") {
		t.Fatalf("sourceRoot missing root error = %v, want missing go.mod error", err)
	}
}

func replaceSourceRootHooks(getwd func() (string, error), stat func(string) (os.FileInfo, error), fatalf func(testing.TB, string, ...any)) func() {
	oldGetwd, oldStat, oldFatalf := sourceRootGetwd, sourceRootStat, sourceRootFatalf
	if getwd != nil {
		sourceRootGetwd = getwd
	}
	if stat != nil {
		sourceRootStat = stat
	}
	if fatalf != nil {
		sourceRootFatalf = fatalf
	}
	return func() {
		sourceRootGetwd, sourceRootStat, sourceRootFatalf = oldGetwd, oldStat, oldFatalf
	}
}

func panicMessage(t *testing.T, fn func()) (message string) {
	t.Helper()
	defer func() {
		if recovered := recover(); recovered != nil {
			message = fmt.Sprint(recovered)
		}
	}()
	fn()
	t.Fatal("function did not panic")
	return ""
}
