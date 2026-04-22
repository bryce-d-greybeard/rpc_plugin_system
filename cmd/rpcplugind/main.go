package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"rpc_plugin_system/internal/adminrpc"
	"rpc_plugin_system/internal/kernel"
)

func main() {
	var (
		runtimeDir = flag.String("runtime-dir", filepath.Join(os.TempDir(), "rpc_plugin_system"), "runtime directory")
		pluginsArg = flag.String("plugins", "", "comma-separated plugin specs in the form id=path")
		pluginPath = flag.String("plugin", "", "path to plugin executable (single-plugin compatibility mode)")
		pluginID   = flag.String("plugin-id", "echo", "plugin id for single-plugin compatibility mode")
	)
	flag.Parse()

	plugins, err := parsePlugins(*pluginsArg, *pluginID, *pluginPath)
	if err != nil {
		log.Fatal(err)
	}

	host, err := kernel.NewHost(kernel.HostConfig{
		RuntimeDir:     *runtimeDir,
		DialTimeout:    3 * time.Second,
		CallTimeout:    500 * time.Millisecond,
		HeartbeatEvery: 2 * time.Second,
		Plugins:        plugins,
	})
	if err != nil {
		log.Fatal(err)
	}
	defer host.Close()

	if err := host.StartAll(); err != nil {
		log.Fatal(err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	host.MonitorLoop(ctx)
	if err := adminrpc.Serve(ctx, filepath.Join(*runtimeDir, "admin.sock"), host); err != nil {
		log.Fatal(err)
	}
}

var pluginIDPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)

func parsePlugins(pluginsArg, pluginID, pluginPath string) ([]kernel.PluginConfig, error) {
	if pluginsArg != "" {
		parts := strings.Split(pluginsArg, ",")
		plugins := make([]kernel.PluginConfig, 0, len(parts))
		for _, part := range parts {
			part = strings.TrimSpace(part)
			fields := strings.SplitN(part, "=", 2)
			if len(fields) != 2 || fields[0] == "" || fields[1] == "" {
				return nil, fmt.Errorf("invalid plugin spec %q, want id=path", part)
			}
			if err := validatePluginID(fields[0]); err != nil {
				return nil, err
			}
			plugins = append(plugins, kernel.PluginConfig{PluginID: fields[0], PluginPath: fields[1]})
		}
		return plugins, nil
	}
	if pluginPath == "" {
		return nil, fmt.Errorf("-plugin is required when -plugins is not set")
	}
	if err := validatePluginID(pluginID); err != nil {
		return nil, err
	}
	return []kernel.PluginConfig{{PluginID: pluginID, PluginPath: pluginPath}}, nil
}

func validatePluginID(pluginID string) error {
	if pluginID == "" {
		return fmt.Errorf("plugin id is required")
	}
	if !pluginIDPattern.MatchString(pluginID) {
		return fmt.Errorf("invalid plugin id %q: use only letters, digits, dot, underscore, and dash, and start with a letter or digit", pluginID)
	}
	return nil
}
