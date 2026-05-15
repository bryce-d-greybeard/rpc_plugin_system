package main

import (
	"bytes"
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

	"github.com/pelletier/go-toml/v2"
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
		flags          = flag.NewFlagSet(name, flag.ContinueOnError)
		configPath     = flags.String("config", "", "path to rpcplugind TOML config")
		runtimeDir     = flags.String("runtime-dir", filepath.Join(os.TempDir(), "rpc_plugin_system"), "runtime directory")
		adminSocket    = flags.String("admin-socket", "", "admin RPC Unix socket path; defaults to <runtime-dir>/admin.sock")
		pluginsArg     = flags.String("plugins", "", "comma-separated plugin specs in the form id=path")
		pluginPath     = flags.String("plugin", "", "path to plugin executable (single-plugin compatibility mode)")
		pluginID       = flags.String("plugin-id", "echo", "plugin id for single-plugin compatibility mode")
		dialTimeout    = flags.Duration("dial-timeout", 3*time.Second, "plugin RPC dial timeout")
		callTimeout    = flags.Duration("call-timeout", 500*time.Millisecond, "plugin RPC call timeout")
		heartbeatEvery = flags.Duration("heartbeat-every", 2*time.Second, "plugin heartbeat interval")
	)
	logger := log.New(deps.logOutput, "", log.LstdFlags)
	flags.SetOutput(deps.logOutput)
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	explicit := explicitFlags(flags)
	cfg, err := parseDaemonConfig(daemonOptions{
		ConfigPath:     *configPath,
		RuntimeDir:     *runtimeDir,
		AdminSocket:    *adminSocket,
		PluginsArg:     *pluginsArg,
		PluginID:       *pluginID,
		PluginPath:     *pluginPath,
		DialTimeout:    *dialTimeout,
		CallTimeout:    *callTimeout,
		HeartbeatEvery: *heartbeatEvery,
		ExplicitFlags:  explicit,
	})
	if err != nil {
		logger.Print(err)
		return 1
	}

	host, err := deps.newHost(cfg.Host)
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
	if err := deps.serve(ctx, cfg.AdminSocket, host); err != nil {
		logger.Print(err)
		return 1
	}
	return 0
}

type daemonOptions struct {
	ConfigPath     string
	RuntimeDir     string
	AdminSocket    string
	PluginsArg     string
	PluginID       string
	PluginPath     string
	DialTimeout    time.Duration
	CallTimeout    time.Duration
	HeartbeatEvery time.Duration
	ExplicitFlags  map[string]bool
}

type resolvedDaemonConfig struct {
	Host        kernel.HostConfig
	AdminSocket string
}

type tomlDaemonConfig struct {
	Daemon  tomlDaemonSection   `toml:"daemon"`
	Plugins []tomlPluginSection `toml:"plugins"`
}

type tomlDaemonSection struct {
	RuntimeDir     string `toml:"runtime_dir"`
	AdminSocket    string `toml:"admin_socket"`
	DialTimeout    string `toml:"dial_timeout"`
	CallTimeout    string `toml:"call_timeout"`
	HeartbeatEvery string `toml:"heartbeat_every"`
}

type tomlPluginSection struct {
	ID   string `toml:"id"`
	Path string `toml:"path"`
}

func parseDaemonConfig(opts daemonOptions) (resolvedDaemonConfig, error) {
	fileCfg, err := loadTOMLConfig(opts.ConfigPath)
	if err != nil {
		return resolvedDaemonConfig{}, err
	}

	runtimeDir := chooseString(opts.ExplicitFlags, "runtime-dir", opts.RuntimeDir, fileCfg.Daemon.RuntimeDir, defaultRuntimeDir(opts.RuntimeDir))
	adminSocket := chooseString(opts.ExplicitFlags, "admin-socket", opts.AdminSocket, fileCfg.Daemon.AdminSocket, "")
	dialTimeout, err := chooseDuration(opts.ExplicitFlags, "dial-timeout", opts.DialTimeout, fileCfg.Daemon.DialTimeout, defaultDuration(opts.DialTimeout, 3*time.Second))
	if err != nil {
		return resolvedDaemonConfig{}, fmt.Errorf("dial_timeout: %w", err)
	}
	callTimeout, err := chooseDuration(opts.ExplicitFlags, "call-timeout", opts.CallTimeout, fileCfg.Daemon.CallTimeout, defaultDuration(opts.CallTimeout, 500*time.Millisecond))
	if err != nil {
		return resolvedDaemonConfig{}, fmt.Errorf("call_timeout: %w", err)
	}
	heartbeatEvery, err := chooseDuration(opts.ExplicitFlags, "heartbeat-every", opts.HeartbeatEvery, fileCfg.Daemon.HeartbeatEvery, defaultDuration(opts.HeartbeatEvery, 2*time.Second))
	if err != nil {
		return resolvedDaemonConfig{}, fmt.Errorf("heartbeat_every: %w", err)
	}
	plugins, err := resolvePlugins(opts, fileCfg.Plugins)
	if err != nil {
		return resolvedDaemonConfig{}, err
	}
	if adminSocket == "" {
		adminSocket = filepath.Join(runtimeDir, "admin.sock")
	}
	return resolvedDaemonConfig{
		Host: kernel.HostConfig{
			RuntimeDir:     runtimeDir,
			DialTimeout:    dialTimeout,
			CallTimeout:    callTimeout,
			HeartbeatEvery: heartbeatEvery,
			Plugins:        plugins,
		},
		AdminSocket: adminSocket,
	}, nil
}

func loadTOMLConfig(path string) (tomlDaemonConfig, error) {
	if path == "" {
		return tomlDaemonConfig{}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return tomlDaemonConfig{}, fmt.Errorf("read config %s: %w", path, err)
	}
	var cfg tomlDaemonConfig
	decoder := toml.NewDecoder(bytes.NewReader(data)).DisallowUnknownFields()
	if err := decoder.Decode(&cfg); err != nil {
		return tomlDaemonConfig{}, fmt.Errorf("parse config %s: %w", path, err)
	}
	return cfg, nil
}

func resolvePlugins(opts daemonOptions, filePlugins []tomlPluginSection) ([]kernel.PluginConfig, error) {
	if opts.ExplicitFlags["plugins"] {
		return parsePlugins(opts.PluginsArg, opts.PluginID, opts.PluginPath)
	}
	if opts.ExplicitFlags["plugin"] || opts.ExplicitFlags["plugin-id"] {
		return parsePlugins("", opts.PluginID, opts.PluginPath)
	}
	if len(filePlugins) > 0 {
		plugins := make([]kernel.PluginConfig, 0, len(filePlugins))
		for _, plugin := range filePlugins {
			if plugin.ID == "" || plugin.Path == "" {
				return nil, fmt.Errorf("invalid config plugin, id and path are required")
			}
			if err := kernel.ValidatePluginID(plugin.ID); err != nil {
				return nil, err
			}
			plugins = append(plugins, kernel.PluginConfig{PluginID: plugin.ID, PluginPath: plugin.Path})
		}
		return plugins, nil
	}
	return parsePlugins(opts.PluginsArg, opts.PluginID, opts.PluginPath)
}

func defaultRuntimeDir(value string) string {
	if value != "" {
		return value
	}
	return filepath.Join(os.TempDir(), "rpc_plugin_system")
}

func defaultDuration(value, fallback time.Duration) time.Duration {
	if value != 0 {
		return value
	}
	return fallback
}

func chooseString(explicit map[string]bool, flagName string, flagValue, fileValue, fallback string) string {
	if explicit[flagName] {
		return flagValue
	}
	if fileValue != "" {
		return fileValue
	}
	return fallback
}

func chooseDuration(explicit map[string]bool, flagName string, flagValue time.Duration, fileValue string, fallback time.Duration) (time.Duration, error) {
	if explicit[flagName] {
		return flagValue, nil
	}
	if fileValue != "" {
		d, err := time.ParseDuration(fileValue)
		if err != nil {
			return 0, err
		}
		return d, nil
	}
	return fallback, nil
}

func explicitFlags(flags *flag.FlagSet) map[string]bool {
	explicit := map[string]bool{}
	flags.Visit(func(f *flag.Flag) { explicit[f.Name] = true })
	return explicit
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
