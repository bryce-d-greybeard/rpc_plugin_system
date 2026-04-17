package main

import (
	"os"
	"time"

	plugin "rpc_plugin_system/sdk/go/plugin"
)

type testPlugin struct {
	pluginID     string
	version      string
	generationID uint64
	startedAt    time.Time
}

func (p *testPlugin) Version() string { return p.version }

func (p *testPlugin) Heartbeat(_ plugin.Empty, out *plugin.HeartbeatResponse) error {
	*out = plugin.HeartbeatResponse{
		PluginID:      p.pluginID,
		Version:       p.version,
		GenerationID:  p.generationID,
		UptimeSeconds: int64(time.Since(p.startedAt).Seconds()),
		Status:        plugin.StatusHealthy,
	}
	return nil
}

func (p *testPlugin) Echo(in plugin.EchoRequest, out *plugin.EchoResponse) error {
	*out = plugin.EchoResponse{Message: in.Message}
	return nil
}

func (p *testPlugin) Sleep(in plugin.SleepRequest, _ *plugin.Empty) error {
	time.Sleep(in.Duration)
	return nil
}

func (p *testPlugin) Crash(in plugin.CrashRequest, _ *plugin.Empty) error {
	os.Exit(in.Code)
	return nil
}

func (p *testPlugin) Shutdown(_ plugin.Empty, _ *plugin.Empty) error {
	go func() {
		if marker := os.Getenv("RPC_PLUGIN_SYSTEM_PLUGIN_SHUTDOWN_MARKER"); marker != "" {
			_ = os.WriteFile(marker, []byte("shutdown\n"), 0o600)
		}
		time.Sleep(50 * time.Millisecond)
		os.Exit(0)
	}()
	return nil
}

func main() {
	cfg, err := plugin.LoadConfigFromEnv()
	if err != nil {
		panic(err)
	}
	logger, err := plugin.NewLogger(cfg)
	if err != nil {
		panic(err)
	}
	defer logger.Close()
	_ = logger.Event(plugin.LogEvent{Event: plugin.EventPluginBootStarted, Message: "echo plugin process booting"})
	p := &testPlugin{pluginID: cfg.PluginID, version: "0.1.0", generationID: cfg.GenerationID, startedAt: time.Now()}
	if err := plugin.ServeWithConfig(cfg, p); err != nil {
		_ = logger.Event(plugin.LogEvent{Level: plugin.LogLevelError, Event: plugin.EventPluginBootFailed, Message: "echo plugin serve failed", Error: err.Error()})
		panic(err)
	}
}
