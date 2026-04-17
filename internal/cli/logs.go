package cli

import (
	"encoding/json"
	"fmt"
	"io"

	"rpc_plugin_system/internal/eventlog"
)

// WriteEventsText writes events as concise human-readable lines.
func WriteEventsText(w io.Writer, events []eventlog.Event) error {
	for _, event := range events {
		if _, err := fmt.Fprintln(w, eventlog.FormatText(event)); err != nil {
			return fmt.Errorf("write text event: %w", err)
		}
	}
	return nil
}

// WriteEventsJSON writes events as indented JSON.
func WriteEventsJSON(w io.Writer, events []eventlog.Event) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(events); err != nil {
		return fmt.Errorf("encode events: %w", err)
	}
	return nil
}
