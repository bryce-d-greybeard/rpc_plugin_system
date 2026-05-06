package runtime

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureDirRejectsEmptyPath(t *testing.T) {
	if err := EnsureDir(""); err == nil {
		t.Fatal("expected empty runtime dir rejection")
	}
}

func TestEnsureDirCreatesPrivateDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "runtime")
	if err := EnsureDir(dir); err != nil {
		t.Fatalf("EnsureDir failed: %v", err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() {
		t.Fatalf("expected directory, got %s", info.Mode())
	}
	if got := info.Mode().Perm(); got != dirMode {
		t.Fatalf("expected mode %03o, got %03o", dirMode, got)
	}
}

func TestEnsureDirFixesExistingDirectoryPermissions(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "runtime")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := EnsureDir(dir); err != nil {
		t.Fatalf("EnsureDir failed: %v", err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != dirMode {
		t.Fatalf("expected mode %03o, got %03o", dirMode, got)
	}
}

func TestEnsureDirRejectsExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime")
	if err := os.WriteFile(path, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := EnsureDir(path); err == nil {
		t.Fatal("expected existing file rejection")
	}
}

func TestEnsureDirRejectsPathUnderExistingFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := EnsureDir(filepath.Join(file, "runtime")); err == nil {
		t.Fatal("expected create runtime dir rejection below file")
	}
}

func TestEnsureDirRejectsInaccessibleParent(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can bypass directory permissions")
	}
	parent := filepath.Join(t.TempDir(), "private")
	if err := os.MkdirAll(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(parent, "runtime")
	if err := os.MkdirAll(child, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(parent, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(parent, 0o700) })
	if err := EnsureDir(child); err == nil {
		t.Fatal("expected inaccessible runtime dir rejection")
	}
}

func TestEnsureDirRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "real")
	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "runtime-link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := EnsureDir(link); err == nil {
		t.Fatal("expected symlink runtime dir rejection")
	}
}

func TestSocketAndAuthPaths(t *testing.T) {
	root := filepath.Join("tmp", "runtime")
	pluginID := "plugin-1"
	if got, want := SocketPath(root, pluginID), filepath.Join(root, pluginID+".sock"); got != want {
		t.Fatalf("SocketPath = %q, want %q", got, want)
	}
	if got, want := AuthPath(root, pluginID), filepath.Join(root, pluginID+".auth"); got != want {
		t.Fatalf("AuthPath = %q, want %q", got, want)
	}
}

func TestListenUnixBindsPrivateSocketAndRemovesStaleSocket(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plugin.sock")

	first, err := ListenUnix(path)
	if err != nil {
		t.Fatalf("first ListenUnix failed: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	listener, err := ListenUnix(path)
	if err != nil {
		t.Fatalf("second ListenUnix failed: %v", err)
	}
	defer listener.Close()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != socketMode {
		t.Fatalf("expected socket mode %03o, got %03o", socketMode, got)
	}
}

func TestListenUnixRejectsMissingParent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "plugin.sock")
	if listener, err := ListenUnix(path); err == nil {
		_ = listener.Close()
		t.Fatal("expected missing parent rejection")
	}
}

func TestValidateExecutableRejectsEmptyPath(t *testing.T) {
	if err := ValidateExecutable(""); err == nil {
		t.Fatal("expected empty executable path rejection")
	}
}

func TestValidateExecutableRejectsRelativePath(t *testing.T) {
	if err := ValidateExecutable("./plugin"); err == nil {
		t.Fatal("expected relative path rejection")
	}
}

func TestValidateExecutableRejectsBareCommand(t *testing.T) {
	if err := ValidateExecutable("rpcplugin-echo"); err == nil {
		t.Fatal("expected bare command rejection")
	}
}

func TestValidateExecutableRejectsMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing-plugin")
	if err := ValidateExecutable(path); err == nil {
		t.Fatal("expected missing executable rejection")
	}
}

func TestValidateExecutableRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "plugin-target")
	if err := os.WriteFile(target, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "plugin-link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := ValidateExecutable(link); err == nil {
		t.Fatal("expected symlink rejection")
	}
}

func TestValidateExecutableRejectsDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := ValidateExecutable(dir); err == nil {
		t.Fatal("expected directory rejection")
	}
}

func TestValidateExecutableRejectsNonExecutableFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plugin")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ValidateExecutable(path); err == nil {
		t.Fatal("expected non-executable rejection")
	}
}

func TestValidateExecutableRejectsGroupWritableFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plugin")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o720); err != nil {
		t.Fatal(err)
	}
	if err := ValidateExecutable(path); err == nil {
		t.Fatal("expected group writable executable rejection")
	}
}

func TestValidateExecutableRejectsWorldWritableFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plugin")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := ValidateExecutable(path); err == nil {
		t.Fatal("expected writable executable rejection")
	}
}

func TestValidateExecutableAcceptsAbsoluteExecutable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plugin")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := ValidateExecutable(path); err != nil {
		t.Fatalf("expected valid executable, got %v", err)
	}
}
