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
)

func main() {
	var (
		runtimeDir = flag.String("runtime-dir", filepath.Join(os.TempDir(), "rpc_plugin_system"), "runtime directory")
		level      = flag.String("level", "", "filter logs by level")
		component  = flag.String("component", "", "filter logs by component")
		eventName  = flag.String("event", "", "filter logs by event name")
		pluginID   = flag.String("plugin-id", "", "filter logs by plugin id")
		method     = flag.String("method", "", "filter logs by method")
		limit      = flag.Int("limit", 200, "limit log lines returned")
		format     = flag.String("format", "text", "log format: text or json")
	)
	flag.Parse()

	if flag.NArg() != 1 {
		log.Fatalf("usage: %s [-runtime-dir DIR] [-level LEVEL] [-component COMPONENT] [-event EVENT] [-plugin-id ID] [-method METHOD] [-limit N] [-format text|json] <status|restart|logs>", os.Args[0])
	}

	switch flag.Arg(0) {
	case "status", "restart":
		client, err := adminrpc.Dial(filepath.Join(*runtimeDir, "admin.sock"), 2*time.Second)
		if err != nil {
			log.Fatal(err)
		}
		defer client.Close()

		var state kernel.State
		switch flag.Arg(0) {
		case "status":
			err = client.Call(adminrpc.MethodStatus, adminrpc.Empty{}, &state)
		case "restart":
			err = client.Call(adminrpc.MethodRestart, adminrpc.Empty{}, &state)
		}
		if err != nil {
			log.Fatal(err)
		}
		if err := cli.WriteState(os.Stdout, state); err != nil {
			log.Fatal(err)
		}
	case "logs":
		events, err := eventlog.ReadAll(filepath.Join(*runtimeDir, "events.jsonl"), eventlog.Filters{
			Level:     *level,
			Component: *component,
			Event:     *eventName,
			PluginID:  *pluginID,
			Method:    *method,
			Limit:     *limit,
		})
		if err != nil {
			log.Fatal(err)
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
		fmt.Printf("usage: %s [-runtime-dir DIR] [-level LEVEL] [-component COMPONENT] [-event EVENT] [-plugin-id ID] [-method METHOD] [-limit N] [-format text|json] <status|restart|logs>\n", os.Args[0])
		return
	default:
		log.Fatalf("unknown command %q, want status, restart, or logs", flag.Arg(0))
	}
}
