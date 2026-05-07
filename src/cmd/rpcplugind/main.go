package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"rpc_plugin_system/internal/adminrpc"
	"rpc_plugin_system/internal/kernel"
)

func main() {
	exitProcess(run(os.Args[0], os.Args[1:], daemonDepsForMain()))
}

var (
	exitProcess       = os.Exit
	daemonDepsForMain = productionDaemonDeps
)

type daemonHost interface {
	StartAll() error
	MonitorLoop(context.Context)
	Close() error
}

type daemonDeps struct {
	newHost      func(kernel.HostConfig) (daemonHost, error)
	serve        func(context.Context, string, daemonHost) error
	notifySignal func(context.Context, ...os.Signal) (context.Context, context.CancelFunc)
	logOutput    io.Writer
}

func productionDaemonDeps() daemonDeps {
	return daemonDeps{
		newHost: func(cfg kernel.HostConfig) (daemonHost, error) {
			return kernel.NewHost(cfg)
		},
		serve: func(ctx context.Context, socketPath string, host daemonHost) error {
			kernelHost, ok := host.(*kernel.Host)
			if !ok {
				return fmt.Errorf("daemon host type %T cannot serve admin rpc", host)
			}
			return adminrpc.Serve(ctx, socketPath, kernelHost)
		},
		notifySignal: signal.NotifyContext,
		logOutput:    os.Stderr,
	}
}

func run(name string, args []string, deps daemonDeps) int {
	var (
		flags      = flag.NewFlagSet(name, flag.ContinueOnError)
		runtimeDir = flags.String("runtime-dir", filepath.Join(os.TempDir(), "rpc_plugin_system"), "runtime directory")
		pluginsArg = flags.String("plugins", "", "comma-separated plugin specs in the form id=path")
		pluginPath = flags.String("plugin", "", "path to plugin executable (single-plugin compatibility mode)")
		pluginID   = flags.String("plugin-id", "echo", "plugin id for single-plugin compatibility mode")
	)
	logger := log.New(deps.logOutput, "", log.LstdFlags)
	flags.SetOutput(deps.logOutput)
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	cfg, err := parseDaemonConfig(daemonOptions{
		RuntimeDir: *runtimeDir,
		PluginsArg: *pluginsArg,
		PluginID:   *pluginID,
		PluginPath: *pluginPath,
	})
	if err != nil {
		logger.Print(err)
		return 1
	}

	host, err := deps.newHost(cfg)
	if err != nil {
		logger.Print(err)
		return 1
	}
	defer host.Close()

	if err := host.StartAll(); err != nil {
		logger.Print(err)
		return 1
	}

	ctx, cancel := deps.notifySignal(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	host.MonitorLoop(ctx)
	if err := deps.serve(ctx, filepath.Join(*runtimeDir, "admin.sock"), host); err != nil {
		logger.Print(err)
		return 1
	}
	return 0
}

type daemonOptions struct {
	RuntimeDir string
	PluginsArg string
	PluginID   string
	PluginPath string
}

func parseDaemonConfig(opts daemonOptions) (kernel.HostConfig, error) {
	plugins, err := parsePlugins(opts.PluginsArg, opts.PluginID, opts.PluginPath)
	if err != nil {
		return kernel.HostConfig{}, err
	}
	return kernel.HostConfig{
		RuntimeDir:     opts.RuntimeDir,
		DialTimeout:    3 * time.Second,
		CallTimeout:    500 * time.Millisecond,
		HeartbeatEvery: 2 * time.Second,
		Plugins:        plugins,
	}, nil
}

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
			if err := kernel.ValidatePluginID(fields[0]); err != nil {
				return nil, err
			}
			plugins = append(plugins, kernel.PluginConfig{PluginID: fields[0], PluginPath: fields[1]})
		}
		return plugins, nil
	}
	if pluginPath == "" {
		return nil, fmt.Errorf("-plugin is required when -plugins is not set")
	}
	if err := kernel.ValidatePluginID(pluginID); err != nil {
		return nil, err
	}
	return []kernel.PluginConfig{{PluginID: pluginID, PluginPath: pluginPath}}, nil
}
