package main

import (
	"os"
	"time"

	plugin "rpc_plugin_system/plugin-authoring-sdk/go-runtime/plugin"
	"rpc_plugin_system/plugin-authoring-sdk/test-plugin-api/testpluginapi"
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

var (
	exitProcess     = os.Exit
	serveWithConfig = plugin.ServeWithConfig
	runPlugin       = run
)

func (p *echoPlugin) Crash(in plugin.CrashRequest, _ *plugin.Empty) error {
	exitProcess(in.Code)
	return nil
}

func (p *echoPlugin) Shutdown(_ plugin.Empty, _ *plugin.Empty) error {
	go func() {
		if marker := os.Getenv(testpluginapi.ShutdownMarkerEnv); marker != "" {
			_ = os.WriteFile(marker, []byte("shutdown\n"), 0o600)
		}
		time.Sleep(50 * time.Millisecond)
		exitProcess(0)
	}()
	return nil
}

func run() error {
	cfg, err := plugin.LoadConfigFromEnv()
	if err != nil {
		return err
	}
	logger, err := plugin.NewLogger(cfg)
	if err != nil {
		return err
	}
	defer logger.Close()
	_ = logger.Event(plugin.LogEvent{Event: plugin.EventPluginBootStarted, Message: "echo plugin process booting"})

	p := &echoPlugin{TemplatePlugin: plugin.NewTemplate(cfg, "0.1.0")}
	if err := serveWithConfig(cfg, p); err != nil {
		_ = logger.Event(plugin.LogEvent{Level: plugin.LogLevelError, Event: plugin.EventPluginBootFailed, Message: "echo plugin serve failed", Error: err.Error()})
		return err
	}
	return nil
}

func main() {
	if err := runPlugin(); err != nil {
		panic(err)
	}
}
