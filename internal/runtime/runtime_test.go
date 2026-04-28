package runtime

import (
	"os"
	"path/filepath"
	"testing"
)

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
