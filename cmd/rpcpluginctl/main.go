package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"rpc_plugin_system/internal/adminrpc"
	"rpc_plugin_system/internal/cli"
	"rpc_plugin_system/internal/eventlog"
	"rpc_plugin_system/internal/kernel"
	"rpc_plugin_system/internal/testpluginapi"
)

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		log.Fatalf("usage: %s [-runtime-dir DIR] [-plugin-id ID] [-message TEXT] [-level LEVEL] [-component COMPONENT] [-event EVENT] [-method METHOD] [-limit N] [-format text|json] [-since DURATION] [-reverse] [-summary] <status|plugins|plugin|capabilities|routes|heartbeat|echo|restart|logs>", os.Args[0])
	}
	command, flagArgs := splitCommandArgs(args)
	if command == "" {
		log.Fatalf("usage: %s [-runtime-dir DIR] [-plugin-id ID] [-message TEXT] [-level LEVEL] [-component COMPONENT] [-event EVENT] [-method METHOD] [-limit N] [-format text|json] [-since DURATION] [-reverse] [-summary] <status|plugins|plugin|capabilities|routes|heartbeat|echo|restart|logs>", os.Args[0])
	}

	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	var (
		runtimeDir = fs.String("runtime-dir", filepath.Join(os.TempDir(), "rpc_plugin_system"), "runtime directory")
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
		log.Fatal(err)
	}
	if fs.NArg() != 0 {
		log.Fatalf("unexpected extra args: %v", fs.Args())
	}

	switch command {
	case "status", "plugins", "plugin", "capabilities", "routes", "heartbeat", "echo", "restart":
		client, err := adminrpc.Dial(filepath.Join(*runtimeDir, "admin.sock"), 2*time.Second)
		if err != nil {
			log.Fatal(err)
		}
		defer client.Close()

		switch command {
		case "status", "plugins":
			var state kernel.HostState
			err = client.Call(adminrpc.MethodPlugins, adminrpc.Empty{}, &state)
			if err != nil {
				log.Fatal(err)
			}
			if err := cli.WriteHostState(os.Stdout, state); err != nil {
				log.Fatal(err)
			}
		case "plugin":
			if *pluginID == "" {
				log.Fatal("-plugin-id is required for plugin")
			}
			var state kernel.State
			err = client.Call(adminrpc.MethodPlugin, adminrpc.PluginRequest{PluginID: *pluginID}, &state)
			if err != nil {
				log.Fatal(err)
			}
			if err := cli.WriteState(os.Stdout, state); err != nil {
				log.Fatal(err)
			}
		case "capabilities":
			var caps map[string][]string
			err = client.Call(adminrpc.MethodCapabilities, adminrpc.Empty{}, &caps)
			if err != nil {
				log.Fatal(err)
			}
			if err := cli.WriteCapabilities(os.Stdout, caps); err != nil {
				log.Fatal(err)
			}
		case "routes":
			var routes []kernel.Route
			err = client.Call(adminrpc.MethodRoutes, adminrpc.Empty{}, &routes)
			if err != nil {
				log.Fatal(err)
			}
			if err := cli.WriteAny(os.Stdout, routes); err != nil {
				log.Fatal(err)
			}
		case "heartbeat":
			if *pluginID == "" {
				log.Fatal("-plugin-id is required for heartbeat")
			}
			var hb testpluginapi.HeartbeatResponse
			err = client.Call(adminrpc.MethodHeartbeat, adminrpc.HeartbeatRequest{PluginID: *pluginID}, &hb)
			if err != nil {
				log.Fatal(err)
			}
			if err := cli.WriteAny(os.Stdout, hb); err != nil {
				log.Fatal(err)
			}
		case "echo":
			if *pluginID == "" {
				log.Fatal("-plugin-id is required for echo")
			}
			var out testpluginapi.EchoResponse
			err = client.Call(adminrpc.MethodEcho, adminrpc.EchoRequest{PluginID: *pluginID, Message: *echoMsg}, &out)
			if err != nil {
				log.Fatal(err)
			}
			if err := cli.WriteAny(os.Stdout, out); err != nil {
				log.Fatal(err)
			}
		case "restart":
			if *pluginID == "" {
				log.Fatal("-plugin-id is required for restart")
			}
			var state kernel.State
			err = client.Call(adminrpc.MethodRestart, adminrpc.RestartRequest{PluginID: *pluginID}, &state)
			if err != nil {
				log.Fatal(err)
			}
			if err := cli.WriteState(os.Stdout, state); err != nil {
				log.Fatal(err)
			}
		}
	case "logs":
		logPath, err := resolveLogPath(*runtimeDir, *pluginID)
		if err != nil {
			log.Fatal(err)
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
			filters.Since = time.Now().Add(-*since)
		}
		events, err := eventlog.ReadAll(logPath, filters)
		if err != nil {
			log.Fatal(err)
		}
		if *summary {
			s := eventlog.Summarize(events)
			switch *format {
			case "text":
				if err := cli.WriteSummaryText(os.Stdout, s); err != nil {
					log.Fatal(err)
				}
			case "json":
				if err := cli.WriteSummaryJSON(os.Stdout, s); err != nil {
					log.Fatal(err)
				}
			default:
				log.Fatalf("unknown log format %q, want text or json", *format)
			}
			return
		}
		switch *format {
		case "text":
			if err := cli.WriteEventsText(os.Stdout, events); err != nil {
				log.Fatal(err)
			}
		case "json":
			if err := cli.WriteEventsJSON(os.Stdout, events); err != nil {
				log.Fatal(err)
			}
		default:
			log.Fatalf("unknown log format %q, want text or json", *format)
		}
	case "help", "-h", "--help":
		fmt.Printf("usage: %s [-runtime-dir DIR] [-plugin-id ID] [-message TEXT] [-level LEVEL] [-component COMPONENT] [-event EVENT] [-method METHOD] [-limit N] [-format text|json] [-since DURATION] [-reverse] [-summary] <status|plugins|plugin|capabilities|routes|heartbeat|echo|restart|logs>\n", os.Args[0])
		return
	default:
		log.Fatalf("unknown command %q, want status, plugins, plugin, capabilities, routes, heartbeat, echo, restart, or logs", command)
	}
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
	return rootLog, nil
}
