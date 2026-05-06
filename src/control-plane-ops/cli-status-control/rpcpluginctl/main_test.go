package main

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"rpc_plugin_system/release-packaging-governance/go-build-deps/testroot"
)

func TestCLIAllowsSubcommandFlagsAfterCommand(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "rpcpluginctl")
	cmd := exec.Command("go", "build", "-buildvcs=false", "-o", bin, "./control-plane-ops/cli-status-control/rpcpluginctl")
	cmd.Dir = testroot.SourceRoot(t)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build rpcpluginctl: %v\n%s", err, out)
	}

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
