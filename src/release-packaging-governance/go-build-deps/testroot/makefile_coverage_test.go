package testroot

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSourceMakeCoverageWritesProfileAndSummaryArtifacts(t *testing.T) {
	srcRoot := SourceRoot(t)
	fakeBin := t.TempDir()
	fakeGo := filepath.Join(fakeBin, "go")
	if runtime.GOOS == "windows" {
		fakeGo += ".bat"
	}
	writeFakeGo(t, fakeGo)

	workDir := t.TempDir()
	artifacts := filepath.Join(workDir, "artifacts")

	cmd := exec.Command("make", "-f", filepath.Join(srcRoot, "Makefile"), "coverage")
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(), "PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("make coverage with fake go failed: %v\n%s", err, out)
	}

	profile := filepath.Join(artifacts, "coverage.out")
	profileBytes, err := os.ReadFile(profile)
	if err != nil {
		t.Fatalf("read coverage profile: %v", err)
	}
	if got := string(profileBytes); !strings.Contains(got, "mode: set") {
		t.Fatalf("coverage profile missing cover mode: %q", got)
	}

	summary := filepath.Join(artifacts, "coverage-summary.txt")
	summaryBytes, err := os.ReadFile(summary)
	if err != nil {
		t.Fatalf("read coverage summary: %v", err)
	}
	summaryText := string(summaryBytes)
	for _, want := range []string{
		"ok  \trpc_plugin_system/example\tcoverage: 100.0% of statements",
		"total:\t\t\t(statements)\t100.0%",
	} {
		if !strings.Contains(summaryText, want) {
			t.Fatalf("coverage summary missing %q in:\n%s", want, summaryText)
		}
	}
	if !strings.Contains(string(out), summaryText) {
		t.Fatalf("make coverage did not print written summary; output:\n%s\nsummary:\n%s", out, summaryText)
	}

	for _, name := range []string{"coverage.out.tmp", "coverage-summary.txt.tmp"} {
		if _, err := os.Stat(filepath.Join(artifacts, name)); !os.IsNotExist(err) {
			t.Fatalf("temporary artifact %s still exists or stat failed: %v", name, err)
		}
	}
}

func TestRootMakeCoverageDelegatesToSourceMakefile(t *testing.T) {
	srcRoot := SourceRoot(t)
	repoRoot := filepath.Dir(srcRoot)
	cmd := exec.Command("make", "-n", "coverage")
	cmd.Dir = repoRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("dry-run root make coverage failed: %v\n%s", err, out)
	}
	if got := string(out); !strings.Contains(got, "make -C src coverage") {
		t.Fatalf("root make coverage did not delegate to src; output:\n%s", got)
	}
}

func writeFakeGo(t *testing.T, path string) {
	t.Helper()
	script := `#!/bin/sh
set -eu
if [ "$1" = "test" ]; then
	case "$2" in
		-coverprofile=*) profile=${2#-coverprofile=} ;;
		*) echo "unexpected go test coverprofile arg: $2" >&2; exit 2 ;;
	esac
	shift 2
	if [ "$1" != "./..." ]; then
		echo "unexpected go test package args: $*" >&2
		exit 2
	fi
	mkdir -p "$(dirname "$profile")"
	printf 'mode: set\nexample.go:1.1,1.2 1 1\n' > "$profile"
	printf 'ok  \trpc_plugin_system/example\tcoverage: 100.0%% of statements\n'
	exit 0
fi
if [ "$1" = "tool" ] && [ "$2" = "cover" ]; then
	case "$3" in
		-func=*) profile=${3#-func=} ;;
		*) echo "unexpected go tool cover arg: $3" >&2; exit 2 ;;
	esac
	if [ ! -s "$profile" ]; then
		echo "missing profile $profile" >&2
		exit 2
	fi
	printf 'example.go:1:\tExample\t\t100.0%%\n'
	printf 'total:\t\t\t(statements)\t100.0%%\n'
	exit 0
fi
echo "unexpected go command: $*" >&2
exit 2
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake go: %v", err)
	}
}
