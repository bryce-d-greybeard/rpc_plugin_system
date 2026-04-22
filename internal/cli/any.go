package cli

import (
	"encoding/json"
	"fmt"
	"io"
)

// WriteAny writes an arbitrary response object as indented JSON.
func WriteAny(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return fmt.Errorf("encode response: %w", err)
	}
	return nil
}
