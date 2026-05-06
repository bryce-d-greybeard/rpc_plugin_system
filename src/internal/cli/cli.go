package cli

import (
	"encoding/json"
	"fmt"
	"io"

	"rpc_plugin_system/internal/kernel"
)

// WriteState writes one plugin state as indented JSON.
func WriteState(w io.Writer, state kernel.State) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(state); err != nil {
		return fmt.Errorf("encode state: %w", err)
	}
	return nil
}

// WriteHostState writes the full multi-plugin host state as indented JSON.
func WriteHostState(w io.Writer, state kernel.HostState) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(state); err != nil {
		return fmt.Errorf("encode host state: %w", err)
	}
	return nil
}

// WriteCapabilities writes the capability map as indented JSON.
func WriteCapabilities(w io.Writer, caps map[string][]string) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(caps); err != nil {
		return fmt.Errorf("encode capability map: %w", err)
	}
	return nil
}
