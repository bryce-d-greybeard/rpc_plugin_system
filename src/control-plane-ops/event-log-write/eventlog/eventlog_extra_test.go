package eventlog

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestLoggerPathHandlesNilAndConcreteLogger(t *testing.T) {
	if got := (*Logger)(nil).Path(); got != "" {
		t.Fatalf("nil logger Path() = %q, want empty", got)
	}

	path := filepath.Join(t.TempDir(), "events.jsonl")
	logger, err := New(path)
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}
	defer logger.Close()

	if got := logger.Path(); got != path {
		t.Fatalf("Path() = %q, want %q", got, path)
	}
}

func TestNewWithOptionsReportsOpenFailure(t *testing.T) {
	missingParent := filepath.Join(t.TempDir(), "missing", "events.jsonl")
	_, err := NewWithOptions(missingParent, Options{})
	if err == nil {
		t.Fatal("expected open failure")
	}
	if !strings.Contains(err.Error(), "open event log") {
		t.Fatalf("error = %q, want open event log context", err)
	}
}

func TestWriteAppliesDefaultsAndReportsMarshalFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	logger, err := New(path)
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}
	defer logger.Close()

	if err := logger.Write(Event{Event: EventPluginStarted}); err != nil {
		t.Fatalf("write defaulted event: %v", err)
	}
	events, err := ReadAll(path, Filters{})
	if err != nil {
		t.Fatalf("read defaulted event: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("len(events) = %d, want 1", len(events))
	}
	if events[0].Time.IsZero() {
		t.Fatal("defaulted event time is zero")
	}
	if events[0].Level != LevelInfo {
		t.Fatalf("default level = %q, want %q", events[0].Level, LevelInfo)
	}
	if events[0].Component != ComponentKernel {
		t.Fatalf("default component = %q, want %q", events[0].Component, ComponentKernel)
	}

	err = logger.Write(Event{Event: EventPluginStarted, Details: map[string]any{"bad": func() {}}})
	if err == nil {
		t.Fatal("expected marshal failure")
	}
	if !strings.Contains(err.Error(), "marshal event") {
		t.Fatalf("error = %q, want marshal event context", err)
	}
}

func TestRotateIfNeededNoopsForDisabledOrEmptyPath(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "events-*.jsonl")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer file.Close()

	logger := &Logger{path: file.Name(), file: file, maxBytes: 0, maxBackups: 1}
	if err := logger.rotateIfNeeded(1 << 20); err != nil {
		t.Fatalf("disabled rotateIfNeeded: %v", err)
	}

	logger.path = ""
	logger.maxBytes = 1
	if err := logger.rotateIfNeeded(1 << 20); err != nil {
		t.Fatalf("empty path rotateIfNeeded: %v", err)
	}
}

func TestWriteReportsFileOperationFailures(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	logger, err := NewWithOptions(path, Options{MaxBytes: 1, MaxBackups: 1})
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}
	if err := logger.Close(); err != nil {
		t.Fatalf("close logger: %v", err)
	}

	err = logger.Write(Event{Event: EventPluginStarted})
	if err == nil {
		t.Fatal("expected write failure after close")
	}
	if !strings.Contains(err.Error(), "stat event log") {
		t.Fatalf("error = %q, want stat event log context", err)
	}

	readOnly, err := os.Open(path)
	if err != nil {
		t.Fatalf("open read-only log: %v", err)
	}
	defer readOnly.Close()
	logger = &Logger{path: path, file: readOnly, maxBytes: 0, maxBackups: 1}
	err = logger.Write(Event{Event: EventPluginStarted})
	if err == nil {
		t.Fatal("expected append failure through read-only file")
	}
	if !strings.Contains(err.Error(), "append event") {
		t.Fatalf("error = %q, want append event context", err)
	}

	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open %s: %v", os.DevNull, err)
	}
	defer devNull.Close()
	logger = &Logger{path: os.DevNull, file: devNull, maxBytes: 0, maxBackups: 1}
	err = logger.Write(Event{Event: EventPluginStarted})
	if err == nil {
		t.Fatal("expected sync failure through os.DevNull")
	}
	if !strings.Contains(err.Error(), "sync event log") {
		t.Fatalf("error = %q, want sync event log context", err)
	}
}

func TestRotateIfNeededReportsRotationFailures(t *testing.T) {
	t.Run("open rotated log", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "events.jsonl")
		file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			t.Fatalf("open log: %v", err)
		}
		if _, err := file.WriteString(strings.Repeat("x", 64)); err != nil {
			t.Fatalf("seed log: %v", err)
		}
		if err := os.Remove(path); err != nil {
			t.Fatalf("remove log name: %v", err)
		}
		if err := os.Remove(dir); err != nil {
			t.Fatalf("remove log directory: %v", err)
		}
		logger := &Logger{path: path, file: file, maxBytes: 1, maxBackups: 1}

		err = logger.rotateIfNeeded(1)
		if err == nil {
			t.Fatal("expected open rotated event log failure")
		}
		if !strings.Contains(err.Error(), "open rotated event log") {
			t.Fatalf("error = %q, want open rotated event log context", err)
		}
	})

	t.Run("backup rename", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "events.jsonl")
		file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			t.Fatalf("open log: %v", err)
		}
		defer file.Close()
		if _, err := file.WriteString(strings.Repeat("x", 64)); err != nil {
			t.Fatalf("seed log: %v", err)
		}
		if err := os.WriteFile(backupPath(path, 1), []byte("old"), 0o600); err != nil {
			t.Fatalf("write first backup: %v", err)
		}
		blockedDir := backupPath(path, 2)
		if err := os.Mkdir(blockedDir, 0o700); err != nil {
			t.Fatalf("mkdir second backup placeholder: %v", err)
		}
		if err := os.WriteFile(filepath.Join(blockedDir, "keep"), []byte("x"), 0o600); err != nil {
			t.Fatalf("make second backup placeholder non-empty: %v", err)
		}
		logger := &Logger{path: path, file: file, maxBytes: 1, maxBackups: 2}

		err = logger.rotateIfNeeded(1)
		if err == nil {
			t.Fatal("expected backup rename failure")
		}
		if !strings.Contains(err.Error(), "rotate backup") {
			t.Fatalf("error = %q, want rotate backup context", err)
		}
		if logger.file == file {
			t.Fatal("expected reopenWithErr to replace closed file")
		}
		logger.file.Close()
	})

	t.Run("current log rename", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "events.jsonl")
		file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			t.Fatalf("open log: %v", err)
		}
		defer file.Close()
		if _, err := file.WriteString(strings.Repeat("x", 64)); err != nil {
			t.Fatalf("seed log: %v", err)
		}
		blockedDir := backupPath(path, 1)
		if err := os.Mkdir(blockedDir, 0o700); err != nil {
			t.Fatalf("mkdir first backup placeholder: %v", err)
		}
		if err := os.WriteFile(filepath.Join(blockedDir, "keep"), []byte("x"), 0o600); err != nil {
			t.Fatalf("make first backup placeholder non-empty: %v", err)
		}
		logger := &Logger{path: path, file: file, maxBytes: 1, maxBackups: 1}

		err = logger.rotateIfNeeded(1)
		if err == nil {
			t.Fatal("expected current log rename failure")
		}
		if !strings.Contains(err.Error(), "rotate current log") {
			t.Fatalf("error = %q, want rotate current log context", err)
		}
		if logger.file == file {
			t.Fatal("expected reopenWithErr to replace closed file")
		}
		logger.file.Close()
	})
}

func TestRotationKeepsNewestBackupsAndWritesRotationEvent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	logger, err := NewWithOptions(path, Options{MaxBytes: 220, MaxBackups: 1})
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}
	defer logger.Close()

	for i := 0; i < 6; i++ {
		if err := logger.Write(Event{Time: time.Unix(int64(i+1), 0).UTC(), Level: LevelInfo, Event: EventPluginStarted, Message: strings.Repeat("x", 80)}); err != nil {
			t.Fatalf("write event %d: %v", i, err)
		}
	}

	if _, err := os.Stat(filepath.Join(dir, "events.1.jsonl")); err != nil {
		t.Fatalf("expected newest backup: %v", err)
	}
	events, err := ReadAll(path, Filters{Event: EventLogRotated})
	if err != nil {
		t.Fatalf("read rotation events: %v", err)
	}
	if len(events) == 0 {
		t.Fatal("expected rotation event in current or backup logs")
	}
	last := events[len(events)-1]
	if last.Component != ComponentRuntime || last.Details["path"] != path {
		t.Fatalf("rotation event = %#v, want runtime event with path detail %q", last, path)
	}
}

func TestBackupPathHandlesNamesWithoutExtensions(t *testing.T) {
	if got, want := backupPath("events", 2), "events.2"; got != want {
		t.Fatalf("backupPath without extension = %q, want %q", got, want)
	}
}

func TestReopenWithErrReturnsCauseAndRestoresFileWhenPossible(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	original, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("open original: %v", err)
	}
	if err := original.Close(); err != nil {
		t.Fatalf("close original: %v", err)
	}
	logger := &Logger{path: path, file: original}
	cause := errors.New("rotate failed")

	if err := reopenWithErr(logger, cause); !errors.Is(err, cause) {
		t.Fatalf("reopenWithErr error = %v, want cause %v", err, cause)
	}
	if logger.file == original {
		t.Fatal("expected logger file to be replaced after successful reopen")
	}
	if err := logger.file.Close(); err != nil {
		t.Fatalf("close reopened file: %v", err)
	}
}

func TestReopenWithErrKeepsFileWhenReopenFails(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "events-*.jsonl")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer file.Close()
	logger := &Logger{path: filepath.Join(t.TempDir(), "missing", "events.jsonl"), file: file}
	cause := errors.New("rename failed")

	if err := reopenWithErr(logger, cause); !errors.Is(err, cause) {
		t.Fatalf("reopenWithErr error = %v, want cause %v", err, cause)
	}
	if logger.file != file {
		t.Fatal("expected original file to remain when reopen fails")
	}
}

func TestReadAllReverseLimitAndAllFilters(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	logger, err := New(path)
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}
	defer logger.Close()

	base := time.Unix(10, 0).UTC()
	input := []Event{
		{Time: base, Level: LevelInfo, Component: ComponentKernel, Event: EventPluginStarted, PluginID: "echo", Method: "Start"},
		{Time: base.Add(time.Second), Level: LevelInfo, Component: ComponentKernel, Event: EventPluginStarted, PluginID: "echo", Method: "Start"},
		{Time: base.Add(2 * time.Second), Level: LevelInfo, Component: ComponentKernel, Event: EventPluginStarted, PluginID: "echo", Method: "Start"},
		{Time: base.Add(3 * time.Second), Level: LevelWarn, Component: ComponentKernel, Event: EventPluginStarted, PluginID: "echo", Method: "Start"},
		{Time: base.Add(4 * time.Second), Level: LevelInfo, Component: ComponentRPC, Event: EventPluginStarted, PluginID: "echo", Method: "Start"},
		{Time: base.Add(5 * time.Second), Level: LevelInfo, Component: ComponentKernel, Event: EventRPCFailed, PluginID: "echo", Method: "Start"},
		{Time: base.Add(6 * time.Second), Level: LevelInfo, Component: ComponentKernel, Event: EventPluginStarted, PluginID: "other", Method: "Start"},
		{Time: base.Add(7 * time.Second), Level: LevelInfo, Component: ComponentKernel, Event: EventPluginStarted, PluginID: "echo", Method: "Other"},
	}
	for i, event := range input {
		if err := logger.Write(event); err != nil {
			t.Fatalf("write event %d: %v", i, err)
		}
	}

	events, err := ReadAll(path, Filters{
		Level:     LevelInfo,
		Component: ComponentKernel,
		Event:     EventPluginStarted,
		PluginID:  "echo",
		Method:    "Start",
		Since:     base.Add(time.Second),
		Reverse:   true,
		Limit:     1,
	})
	if err != nil {
		t.Fatalf("read filtered events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("len(events) = %d, want limit-trimmed 1", len(events))
	}
	gotTimes := []time.Time{events[0].Time}
	wantTimes := []time.Time{base.Add(2 * time.Second)}
	if !reflect.DeepEqual(gotTimes, wantTimes) {
		t.Fatalf("filtered times = %v, want %v", gotTimes, wantTimes)
	}
}

func TestReadAllReportsMissingLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.jsonl")
	_, err := ReadAll(path, Filters{})
	if err == nil {
		t.Fatal("expected missing log error")
	}
	if !strings.Contains(err.Error(), "open event log") || !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("error = %v, want wrapped not-exist open event log", err)
	}
}

func TestReadOneAcceptsFinalLineWithoutNewlineAndReportsOpenFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	content := `{"time":"1970-01-01T00:00:01Z","level":"info","component":"kernel","event":"plugin_started"}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write log: %v", err)
	}

	events, err := readOne(path, Filters{})
	if err != nil {
		t.Fatalf("readOne final line: %v", err)
	}
	if len(events) != 1 || events[0].Event != EventPluginStarted {
		t.Fatalf("events = %#v, want one plugin_started", events)
	}

	_, err = readOne(filepath.Join(t.TempDir(), "missing.jsonl"), Filters{})
	if err == nil {
		t.Fatal("expected readOne open failure")
	}
	if !strings.Contains(err.Error(), "open event log") {
		t.Fatalf("error = %q, want open event log context", err)
	}

	_, err = readOne(t.TempDir(), Filters{})
	if err == nil {
		t.Fatal("expected read failure while reading directory")
	}
	if !strings.Contains(err.Error(), "read event log") {
		t.Fatalf("error = %q, want read event log context", err)
	}
}

func TestReverseHandlesEmptySingletonAndMultipleEvents(t *testing.T) {
	var empty []Event
	reverse(empty)
	if len(empty) != 0 {
		t.Fatalf("empty reverse len = %d, want 0", len(empty))
	}

	single := []Event{{Event: "one"}}
	reverse(single)
	if single[0].Event != "one" {
		t.Fatalf("singleton reverse = %#v", single)
	}

	events := []Event{{Event: "one"}, {Event: "two"}, {Event: "three"}}
	reverse(events)
	got := []string{events[0].Event, events[1].Event, events[2].Event}
	want := []string{"three", "two", "one"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("reverse events = %v, want %v", got, want)
	}
}
