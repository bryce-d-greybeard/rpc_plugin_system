package cli

import (
	"encoding/json"
	"fmt"
	"io"

	"rpc_plugin_system/internal/kernel"
)

// WriteState writes the current manager state as indented JSON.
func WriteState(w io.Writer, state kernel.State) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(state); err != nil {
		return fmt.Errorf("encode state: %w", err)
	}
	return nil
}
