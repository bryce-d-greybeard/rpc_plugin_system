package main

import (
	"errors"
	"net"
	"os"
	"time"

	"rpc_plugin_system/internal/testpluginapi"
	plugin "rpc_plugin_system/sdk/go/plugin"
)

type failurePlugin struct {
	pluginID     string
	version      string
	generationID uint64
	startedAt    time.Time
	behavior     testpluginapi.Env
	listener     net.Listener
}

func (p *failurePlugin) Version() string { return p.version }

func (p *failurePlugin) Identity() (string, uint64) {
	pluginID := p.pluginID
	if p.behavior.PluginID != "" {
		pluginID = p.behavior.PluginID
	}
	return pluginID, uint64(int64(p.generationID) + p.behavior.GenerationOffset)
}

func (p *failurePlugin) OnAuthAttempt(_ plugin.AuthRequest) error {
	if p.behavior.FailAuth {
		return errors.New("auth failure requested")
	}
	return nil
}

func (p *failurePlugin) OnServeListener(listener net.Listener) {
	p.listener = listener
	if p.behavior.CloseOnAccept && listener != nil {
		_ = listener.Close()
	}
}

func (p *failurePlugin) Heartbeat(_ plugin.Empty, out *plugin.HeartbeatResponse) error {
	if p.behavior.HeartbeatErrors > 0 {
		p.behavior.HeartbeatErrors--
		return errors.New("heartbeat failure requested")
	}
	pluginID := p.pluginID
	if p.behavior.PluginID != "" {
		pluginID = p.behavior.PluginID
	}
	*out = plugin.HeartbeatResponse{
		PluginID:      pluginID,
		Version:       p.version,
		GenerationID:  uint64(int64(p.generationID) + p.behavior.GenerationOffset),
		UptimeSeconds: int64(time.Since(p.startedAt).Seconds()),
		Status:        plugin.Status(p.behavior.HeartbeatStatus),
	}
	return nil
}

func (p *failurePlugin) Echo(in plugin.EchoRequest, out *plugin.EchoResponse) error {
	if p.behavior.CrashOnEcho {
		os.Exit(9)
	}
	if p.behavior.CloseOnEcho && p.listener != nil {
		_ = p.listener.Close()
	}
	*out = plugin.EchoResponse{Message: in.Message}
	return nil
}

func (p *failurePlugin) Sleep(in plugin.SleepRequest, _ *plugin.Empty) error {
	scale := p.behavior.SleepScale
	if scale <= 0 {
		scale = 1
	}
	time.Sleep(time.Duration(float64(in.Duration) * scale))
	return nil
}

func (p *failurePlugin) Crash(in plugin.CrashRequest, _ *plugin.Empty) error {
	os.Exit(in.Code)
	return nil
}

func (p *failurePlugin) Shutdown(_ plugin.Empty, _ *plugin.Empty) error {
	go func() {
		if marker := os.Getenv(testpluginapi.ShutdownMarkerEnv); marker != "" {
			_ = os.WriteFile(marker, []byte("shutdown\n"), 0o600)
		}
		time.Sleep(p.behavior.ShutdownDelay())
		os.Exit(0)
	}()
	return nil
}

func (p *failurePlugin) Capabilities() []string {
	if p.behavior.CapabilitiesMode == "error" {
		return []string{"__capabilities_error__"}
	}
	if p.behavior.CapabilitiesMode == "empty" {
		return nil
	}
	return []string{"heartbeat", "shutdown", "echo", "sleep", "crash"}
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
	behavior := testpluginapi.LoadConfig()
	_ = logger.Event(plugin.LogEvent{Event: plugin.EventPluginBootStarted, Message: "failure plugin process booting", Details: map[string]any{"behavior_version": behavior.Version}})
	p := &failurePlugin{pluginID: cfg.PluginID, version: behavior.Version, generationID: cfg.GenerationID, startedAt: time.Now(), behavior: behavior}
	if err := plugin.ServeWithConfig(cfg, p); err != nil {
		_ = logger.Event(plugin.LogEvent{Level: plugin.LogLevelError, Event: plugin.EventPluginBootFailed, Message: "failure plugin serve failed", Error: err.Error()})
		panic(err)
	}
}
