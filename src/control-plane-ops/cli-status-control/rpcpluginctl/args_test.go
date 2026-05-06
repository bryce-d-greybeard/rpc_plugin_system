package main

import (
	"reflect"
	"testing"
)

func TestSplitCommandArgs(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantCmd    string
		wantOthers []string
	}{
		{
			name:       "flags before command",
			args:       []string{"-runtime-dir", "/tmp/demo", "status"},
			wantCmd:    "status",
			wantOthers: []string{"-runtime-dir", "/tmp/demo"},
		},
		{
			name:       "flags after command",
			args:       []string{"logs", "-limit", "5"},
			wantCmd:    "logs",
			wantOthers: []string{"-limit", "5"},
		},
		{
			name:       "equals style flag",
			args:       []string{"-runtime-dir=/tmp/demo", "logs", "-summary=true"},
			wantCmd:    "logs",
			wantOthers: []string{"-runtime-dir=/tmp/demo", "-summary=true"},
		},
		{
			name:       "double dash separator",
			args:       []string{"-runtime-dir", "/tmp/demo", "--", "routes"},
			wantCmd:    "routes",
			wantOthers: []string{"-runtime-dir", "/tmp/demo"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotCmd, gotOthers := splitCommandArgs(tt.args)
			if gotCmd != tt.wantCmd {
				t.Fatalf("command = %q want %q", gotCmd, tt.wantCmd)
			}
			if !reflect.DeepEqual(gotOthers, tt.wantOthers) {
				t.Fatalf("other args = %#v want %#v", gotOthers, tt.wantOthers)
			}
		})
	}
}
