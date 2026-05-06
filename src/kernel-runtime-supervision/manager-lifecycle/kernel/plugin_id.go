package kernel

import (
	"fmt"
	"regexp"
)

var pluginIDPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)

// ValidatePluginID rejects empty or path-like plugin identifiers before they are
// used in runtime paths or routing requests.
func ValidatePluginID(pluginID string) error {
	if pluginID == "" {
		return fmt.Errorf("plugin id is required")
	}
	if !pluginIDPattern.MatchString(pluginID) {
		return fmt.Errorf("invalid plugin id %q: use only letters, digits, dot, underscore, and dash, and start with a letter or digit", pluginID)
	}
	return nil
}
