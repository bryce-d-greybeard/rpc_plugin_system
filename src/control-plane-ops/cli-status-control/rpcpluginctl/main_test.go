package main

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"rpc_plugin_system/release-packaging-governance/go-build-deps/testroot"
)

func TestCLIAllowsSubcommandFlagsAfterCommand(t *testing.T) {
	bin := buildCLI(t)

	run := exec.Command(bin, "logs", "-limit", "5")
	out, err := run.CombinedOutput()
	if err == nil {
		t.Fatalf("expected logs command to fail without runtime data")
	}
	text := string(out)
	if strings.Contains(text, "usage:") {
		t.Fatalf("expected logs subcommand flags to parse, got usage output: %s", text)
	}
	if !strings.Contains(text, "no such file or directory") && !strings.Contains(text, "open event log") {
		t.Fatalf("expected parsed logs command to attempt log access, got: %s", text)
	}
}

func TestCLIValidatesLogFlagsBeforeIO(t *testing.T) {
	bin := buildCLI(t)
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{name: "bad format", args: []string{"logs", "-format", "xml"}, want: "unknown log format"},
		{name: "negative limit", args: []string{"logs", "-limit", "-1"}, want: "-limit must be non-negative"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command(bin, tc.args...)
			out, err := cmd.CombinedOutput()
			if err == nil {
				t.Fatalf("expected validation failure")
			}
			text := string(out)
			if !strings.Contains(text, tc.want) {
				t.Fatalf("output = %q, want substring %q", text, tc.want)
			}
			if strings.Contains(text, "open event log") || strings.Contains(text, "no such file or directory") {
				t.Fatalf("validation happened after log IO: %s", text)
			}
		})
	}
}

func TestCLIValidatesPluginIDBeforeAdminDialOrLogIO(t *testing.T) {
	bin := buildCLI(t)
	for _, tc := range []struct {
		name string
		args []string
	}{
		{name: "admin command", args: []string{"-plugin-id", "../echo", "plugin"}},
		{name: "logs command", args: []string{"logs", "-plugin-id", "echo/test"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command(bin, tc.args...)
			out, err := cmd.CombinedOutput()
			if err == nil {
				t.Fatalf("expected validation failure")
			}
			text := string(out)
			if !strings.Contains(text, "invalid plugin id") {
				t.Fatalf("output = %q, want invalid plugin id", text)
			}
			if strings.Contains(text, "dial admin socket") || strings.Contains(text, "open event log") || strings.Contains(text, "no such file or directory") {
				t.Fatalf("plugin id validation happened after IO: %s", text)
			}
		})
	}
}

func buildCLI(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "rpcpluginctl")
	cmd := exec.Command("go", "build", "-buildvcs=false", "-o", bin, "./control-plane-ops/cli-status-control/rpcpluginctl")
	cmd.Dir = testroot.SourceRoot(t)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build rpcpluginctl: %v\n%s", err, out)
	}
	return bin
}
