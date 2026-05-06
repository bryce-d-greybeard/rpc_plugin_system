package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateOptions(t *testing.T) {
	for _, tt := range []struct {
		name     string
		command  string
		pluginID string
		limit    int
		format   string
		wantErr  string
	}{
		{name: "admin command requires plugin id", command: "plugin", limit: 1, format: "text", wantErr: "-plugin-id is required for plugin"},
		{name: "heartbeat validates plugin id", command: "heartbeat", pluginID: "bad/id", limit: 1, format: "text", wantErr: "invalid plugin id"},
		{name: "echo accepts plugin id", command: "echo", pluginID: "echo_1.2", limit: 1, format: "text"},
		{name: "logs accepts empty plugin id", command: "logs", limit: 0, format: "json"},
		{name: "logs rejects invalid plugin id", command: "logs", pluginID: "../echo", limit: 1, format: "text", wantErr: "invalid plugin id"},
		{name: "logs rejects negative limit", command: "logs", limit: -1, format: "text", wantErr: "-limit must be non-negative"},
		{name: "logs rejects unknown format", command: "logs", limit: 1, format: "yaml", wantErr: "unknown log format"},
		{name: "non logs ignores format", command: "status", limit: -1, format: "yaml"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := validateOptions(tt.command, tt.pluginID, tt.limit, tt.format)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validateOptions() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("validateOptions() error = %v, want substring %q", err, tt.wantErr)
			}
		})
	}
}

func TestResolveLogPath(t *testing.T) {
	t.Run("plugin id uses plugin log without stat", func(t *testing.T) {
		runtimeDir := t.TempDir()
		got, err := resolveLogPath(runtimeDir, "echo")
		if err != nil {
			t.Fatalf("resolveLogPath: %v", err)
		}
		want := filepath.Join(runtimeDir, "echo", "events.jsonl")
		if got != want {
			t.Fatalf("path = %q want %q", got, want)
		}
	})

	t.Run("root log wins", func(t *testing.T) {
		runtimeDir := t.TempDir()
		rootLog := filepath.Join(runtimeDir, "events.jsonl")
		if err := os.WriteFile(rootLog, []byte("{}\n"), 0o600); err != nil {
			t.Fatalf("write root log: %v", err)
		}
		if err := os.Mkdir(filepath.Join(runtimeDir, "echo"), 0o700); err != nil {
			t.Fatalf("mkdir plugin: %v", err)
		}
		if err := os.WriteFile(filepath.Join(runtimeDir, "echo", "events.jsonl"), []byte("{}\n"), 0o600); err != nil {
			t.Fatalf("write plugin log: %v", err)
		}
		got, err := resolveLogPath(runtimeDir, "")
		if err != nil {
			t.Fatalf("resolveLogPath: %v", err)
		}
		if got != rootLog {
			t.Fatalf("path = %q want %q", got, rootLog)
		}
	})

	t.Run("single plugin log fallback", func(t *testing.T) {
		runtimeDir := t.TempDir()
		pluginDir := filepath.Join(runtimeDir, "echo")
		if err := os.Mkdir(pluginDir, 0o700); err != nil {
			t.Fatalf("mkdir plugin: %v", err)
		}
		pluginLog := filepath.Join(pluginDir, "events.jsonl")
		if err := os.WriteFile(pluginLog, []byte("{}\n"), 0o600); err != nil {
			t.Fatalf("write plugin log: %v", err)
		}
		if err := os.WriteFile(filepath.Join(runtimeDir, "plain-file"), nil, 0o600); err != nil {
			t.Fatalf("write non-dir: %v", err)
		}
		got, err := resolveLogPath(runtimeDir, "")
		if err != nil {
			t.Fatalf("resolveLogPath: %v", err)
		}
		if got != pluginLog {
			t.Fatalf("path = %q want %q", got, pluginLog)
		}
	})

	t.Run("multiple plugin logs require plugin id", func(t *testing.T) {
		runtimeDir := t.TempDir()
		for _, id := range []string{"echo", "clock"} {
			dir := filepath.Join(runtimeDir, id)
			if err := os.Mkdir(dir, 0o700); err != nil {
				t.Fatalf("mkdir %s: %v", id, err)
			}
			if err := os.WriteFile(filepath.Join(dir, "events.jsonl"), []byte("{}\n"), 0o600); err != nil {
				t.Fatalf("write %s log: %v", id, err)
			}
		}
		got, err := resolveLogPath(runtimeDir, "")
		if err == nil || !strings.Contains(err.Error(), "multiple plugin logs found") {
			t.Fatalf("resolveLogPath() = %q, %v; want multiple logs error", got, err)
		}
	})

	t.Run("empty runtime dir returns root log path", func(t *testing.T) {
		runtimeDir := t.TempDir()
		got, err := resolveLogPath(runtimeDir, "")
		if err != nil {
			t.Fatalf("resolveLogPath: %v", err)
		}
		want := filepath.Join(runtimeDir, "events.jsonl")
		if got != want {
			t.Fatalf("path = %q want %q", got, want)
		}
	})

	t.Run("missing runtime dir returns root path and read error", func(t *testing.T) {
		runtimeDir := filepath.Join(t.TempDir(), "missing")
		got, err := resolveLogPath(runtimeDir, "")
		if err == nil {
			t.Fatalf("expected read dir error")
		}
		want := filepath.Join(runtimeDir, "events.jsonl")
		if got != want {
			t.Fatalf("path = %q want %q", got, want)
		}
	})
}
