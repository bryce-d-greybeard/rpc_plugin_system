package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"rpc_plugin_system/control-plane-ops/admin-rpc/adminrpc"
	"rpc_plugin_system/control-plane-ops/cli-status-control/cli"
	"rpc_plugin_system/control-plane-ops/event-log-write/eventlog"
	"rpc_plugin_system/kernel-runtime-supervision/manager-lifecycle/kernel"
	"rpc_plugin_system/plugin-authoring-sdk/test-plugin-api/testpluginapi"
)

var exit = os.Exit

func main() {
	exit(run(os.Args[1:], runEnv{Prog: os.Args[0], TempDir: os.TempDir, Now: time.Now}, os.Stdout, os.Stderr))
}

type runEnv struct {
	Prog    string
	TempDir func() string
	Now     func() time.Time
}

func (e runEnv) normalize() runEnv {
	if e.Prog == "" {
		e.Prog = "rpcpluginctl"
	}
	if e.TempDir == nil {
		e.TempDir = os.TempDir
	}
	if e.Now == nil {
		e.Now = time.Now
	}
	return e
}

func run(args []string, env runEnv, stdout, stderr io.Writer) int {
	env = env.normalize()
	logger := log.New(stderr, log.Prefix(), log.Flags())
	usage := func() string {
		return fmt.Sprintf("usage: %s [-runtime-dir DIR] [-plugin-id ID] [-message TEXT] [-level LEVEL] [-component COMPONENT] [-event EVENT] [-method METHOD] [-limit N] [-format text|json] [-since DURATION] [-reverse] [-summary] <status|plugins|plugin|capabilities|routes|heartbeat|echo|restart|logs>", env.Prog)
	}
	fatal := func(v ...any) int {
		logger.Print(v...)
		return 1
	}
	fatalf := func(format string, v ...any) int {
		logger.Printf(format, v...)
		return 1
	}
	if len(args) == 0 {
		return fatal(usage())
	}
	command, flagArgs := splitCommandArgs(args)
	if command == "" {
		return fatal(usage())
	}

	fs := flag.NewFlagSet(env.Prog, flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		runtimeDir = fs.String("runtime-dir", filepath.Join(env.TempDir(), "rpc_plugin_system"), "runtime directory")
		pluginID   = fs.String("plugin-id", "", "target plugin id for plugin-specific operations")
		echoMsg    = fs.String("message", "ping", "message for echo command")
		level      = fs.String("level", "", "filter logs by level")
		component  = fs.String("component", "", "filter logs by component")
		eventName  = fs.String("event", "", "filter logs by event name")
		method     = fs.String("method", "", "filter logs by method")
		limit      = fs.Int("limit", 200, "limit log lines returned")
		format     = fs.String("format", "text", "log format: text or json")
		since      = fs.Duration("since", 0, "only show logs newer than this duration, e.g. 15m")
		reverse    = fs.Bool("reverse", true, "show newest matching logs first")
		summary    = fs.Bool("summary", false, "show summary counts instead of raw log lines")
	)
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		return fatalf("unexpected extra args: %v", fs.Args())
	}
	if err := validateOptions(command, *pluginID, *limit, *format); err != nil {
		return fatal(err)
	}

	switch command {
	case "status", "plugins", "plugin", "capabilities", "routes", "heartbeat", "echo", "restart":
		client, err := adminrpc.Dial(filepath.Join(*runtimeDir, "admin.sock"), 2*time.Second)
		if err != nil {
			return fatal(err)
		}
		defer client.Close()

		switch command {
		case "status", "plugins":
			var state kernel.HostState
			err = client.Call(adminrpc.MethodPlugins, adminrpc.Empty{}, &state)
			if err != nil {
				return fatal(err)
			}
			if err := cli.WriteHostState(stdout, state); err != nil {
				return fatal(err)
			}
		case "plugin":
			var state kernel.State
			err = client.Call(adminrpc.MethodPlugin, adminrpc.PluginRequest{PluginID: *pluginID}, &state)
			if err != nil {
				return fatal(err)
			}
			if err := cli.WriteState(stdout, state); err != nil {
				return fatal(err)
			}
		case "capabilities":
			var caps map[string][]string
			err = client.Call(adminrpc.MethodCapabilities, adminrpc.Empty{}, &caps)
			if err != nil {
				return fatal(err)
			}
			if err := cli.WriteCapabilities(stdout, caps); err != nil {
				return fatal(err)
			}
		case "routes":
			var routes []kernel.Route
			err = client.Call(adminrpc.MethodRoutes, adminrpc.Empty{}, &routes)
			if err != nil {
				return fatal(err)
			}
			if err := cli.WriteAny(stdout, routes); err != nil {
				return fatal(err)
			}
		case "heartbeat":
			var hb testpluginapi.HeartbeatResponse
			err = client.Call(adminrpc.MethodHeartbeat, adminrpc.HeartbeatRequest{PluginID: *pluginID}, &hb)
			if err != nil {
				return fatal(err)
			}
			if err := cli.WriteAny(stdout, hb); err != nil {
				return fatal(err)
			}
		case "echo":
			var out testpluginapi.EchoResponse
			err = client.Call(adminrpc.MethodEcho, adminrpc.EchoRequest{PluginID: *pluginID, Message: *echoMsg}, &out)
			if err != nil {
				return fatal(err)
			}
			if err := cli.WriteAny(stdout, out); err != nil {
				return fatal(err)
			}
		case "restart":
			var state kernel.State
			err = client.Call(adminrpc.MethodRestart, adminrpc.RestartRequest{PluginID: *pluginID}, &state)
			if err != nil {
				return fatal(err)
			}
			if err := cli.WriteState(stdout, state); err != nil {
				return fatal(err)
			}
		}
	case "logs":
		logPath, err := resolveLogPath(*runtimeDir, *pluginID)
		if err != nil {
			return fatal(err)
		}
		filters := eventlog.Filters{
			Level:     *level,
			Component: *component,
			Event:     *eventName,
			PluginID:  *pluginID,
			Method:    *method,
			Limit:     *limit,
			Reverse:   *reverse,
		}
		if *since > 0 {
			filters.Since = env.Now().Add(-*since)
		}
		events, err := eventlog.ReadAll(logPath, filters)
		if err != nil {
			return fatal(err)
		}
		if *summary {
			s := eventlog.Summarize(events)
			if *format == "text" {
				if err := cli.WriteSummaryText(stdout, s); err != nil {
					return fatal(err)
				}
				return 0
			}
			if err := cli.WriteSummaryJSON(stdout, s); err != nil {
				return fatal(err)
			}
			return 0
		}
		if *format == "text" {
			if err := cli.WriteEventsText(stdout, events); err != nil {
				return fatal(err)
			}
			return 0
		}
		if err := cli.WriteEventsJSON(stdout, events); err != nil {
			return fatal(err)
		}
	case "help", "-h", "--help":
		fmt.Fprintf(stdout, "%s\n", usage())
		return 0
	default:
		return fatalf("unknown command %q, want status, plugins, plugin, capabilities, routes, heartbeat, echo, restart, or logs", command)
	}
	return 0
}

func validateOptions(command, pluginID string, limit int, format string) error {
	switch command {
	case "plugin", "heartbeat", "echo", "restart":
		if pluginID == "" {
			return fmt.Errorf("-plugin-id is required for %s", command)
		}
	}
	if pluginID != "" {
		if err := kernel.ValidatePluginID(pluginID); err != nil {
			return err
		}
	}
	if command == "logs" {
		if limit < 0 {
			return fmt.Errorf("-limit must be non-negative")
		}
		switch format {
		case "text", "json":
		default:
			return fmt.Errorf("unknown log format %q, want text or json", format)
		}
	}
	return nil
}

func resolveLogPath(runtimeDir, pluginID string) (string, error) {
	if pluginID != "" {
		return filepath.Join(runtimeDir, pluginID, "events.jsonl"), nil
	}
	rootLog := filepath.Join(runtimeDir, "events.jsonl")
	if _, err := os.Stat(rootLog); err == nil {
		return rootLog, nil
	}
	entries, err := os.ReadDir(runtimeDir)
	if err != nil {
		return rootLog, err
	}
	var pluginLogs []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		candidate := filepath.Join(runtimeDir, entry.Name(), "events.jsonl")
		if _, err := os.Stat(candidate); err == nil {
			pluginLogs = append(pluginLogs, candidate)
		}
	}
	if len(pluginLogs) == 1 {
		return pluginLogs[0], nil
	}
	if len(pluginLogs) > 1 {
		return "", fmt.Errorf("multiple plugin logs found under %s, use -plugin-id", runtimeDir)
	}
	return rootLog, nil
}
