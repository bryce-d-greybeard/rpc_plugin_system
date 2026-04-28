package main

import (
	"os"
	"time"

	"rpc_plugin_system/internal/testpluginapi"
	plugin "rpc_plugin_system/sdk/go/plugin"
)

type echoPlugin struct {
	*plugin.TemplatePlugin
}

func (p *echoPlugin) Echo(in plugin.EchoRequest, out *plugin.EchoResponse) error {
	*out = plugin.EchoResponse{Message: in.Message}
	return nil
}

func (p *echoPlugin) Sleep(in plugin.SleepRequest, _ *plugin.Empty) error {
	time.Sleep(in.Duration)
	return nil
}

func (p *echoPlugin) Crash(in plugin.CrashRequest, _ *plugin.Empty) error {
	os.Exit(in.Code)
	return nil
}

func (p *echoPlugin) Shutdown(_ plugin.Empty, _ *plugin.Empty) error {
	go func() {
		if marker := os.Getenv(testpluginapi.ShutdownMarkerEnv); marker != "" {
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

	p := &echoPlugin{TemplatePlugin: plugin.NewTemplate(cfg, "0.1.0")}
	if err := plugin.ServeWithConfig(cfg, p); err != nil {
		_ = logger.Event(plugin.LogEvent{Level: plugin.LogLevelError, Event: plugin.EventPluginBootFailed, Message: "echo plugin serve failed", Error: err.Error()})
		panic(err)
	}
}
