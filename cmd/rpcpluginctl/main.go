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
	var (
		runtimeDir = flag.String("runtime-dir", filepath.Join(os.TempDir(), "rpc_plugin_system"), "runtime directory")
		pluginID   = flag.String("plugin-id", "", "target plugin id for plugin-specific operations")
		echoMsg    = flag.String("message", "ping", "message for echo command")
		level      = flag.String("level", "", "filter logs by level")
		component  = flag.String("component", "", "filter logs by component")
		eventName  = flag.String("event", "", "filter logs by event name")
		method     = flag.String("method", "", "filter logs by method")
		limit      = flag.Int("limit", 200, "limit log lines returned")
		format     = flag.String("format", "text", "log format: text or json")
		since      = flag.Duration("since", 0, "only show logs newer than this duration, e.g. 15m")
		reverse    = flag.Bool("reverse", true, "show newest matching logs first")
		summary    = flag.Bool("summary", false, "show summary counts instead of raw log lines")
	)
	flag.Parse()

	if flag.NArg() != 1 {
		log.Fatalf("usage: %s [-runtime-dir DIR] [-plugin-id ID] [-message TEXT] [-level LEVEL] [-component COMPONENT] [-event EVENT] [-method METHOD] [-limit N] [-format text|json] [-since DURATION] [-reverse] [-summary] <status|plugins|plugin|capabilities|routes|heartbeat|echo|restart|logs>", os.Args[0])
	}

	switch flag.Arg(0) {
	case "status", "plugins", "plugin", "capabilities", "routes", "heartbeat", "echo", "restart":
		client, err := adminrpc.Dial(filepath.Join(*runtimeDir, "admin.sock"), 2*time.Second)
		if err != nil {
			log.Fatal(err)
		}
		defer client.Close()

		switch flag.Arg(0) {
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
		logPath := filepath.Join(*runtimeDir, "events.jsonl")
		if *pluginID != "" {
			logPath = filepath.Join(*runtimeDir, *pluginID, "events.jsonl")
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
		log.Fatalf("unknown command %q, want status, plugins, plugin, capabilities, routes, heartbeat, echo, restart, or logs", flag.Arg(0))
	}
}
