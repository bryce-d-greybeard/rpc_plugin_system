package plugin_test

import (
	"path/filepath"

	plugin "rpc_plugin_system/pkg/plugin"
)

func ExampleLogger_ProviderDiagnostic() {
	cfg := plugin.Config{SocketPath: filepath.Join("runtime", "plugin.sock"), PluginID: "provider.echo", GenerationID: 7}
	logger, err := plugin.NewLogger(cfg)
	if err != nil {
		panic(err)
	}
	defer logger.Close()

	if err := logger.ProviderDiagnostic(plugin.ProviderDiagnostic{
		CapabilityID:  "mail.send",
		OperationID:   "send_message",
		CorrelationID: "corr-123",
		Status:        plugin.ProviderStatusDegraded,
		DurationMS:    42,
		ErrorClass:    "upstream_unavailable",
		Message:       "provider degraded",
	}); err != nil {
		panic(err)
	}
}
