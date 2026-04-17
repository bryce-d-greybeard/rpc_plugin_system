package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"rpc_plugin_system/internal/adminrpc"
	"rpc_plugin_system/internal/kernel"
)

func main() {
	var (
		runtimeDir = flag.String("runtime-dir", filepath.Join(os.TempDir(), "rpc_plugin_system"), "runtime directory")
		pluginPath = flag.String("plugin", "", "path to plugin executable")
		pluginID   = flag.String("plugin-id", "echo", "plugin id")
	)
	flag.Parse()

	if *pluginPath == "" {
		log.Fatal("-plugin is required")
	}
	manager, err := kernel.New(kernel.Config{
		RuntimeDir:       *runtimeDir,
		PluginPath:       *pluginPath,
		PluginID:         *pluginID,
			DialTimeout:      3 * time.Second,
		CallTimeout:      500 * time.Millisecond,
		HeartbeatEvery:   2 * time.Second,
		EventLogPath:     filepath.Join(*runtimeDir, "events.jsonl"),
	})
	if err != nil {
		log.Fatal(err)
	}
	defer manager.Close()

	if err := manager.Start(); err != nil {
		log.Fatal(err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	go manager.MonitorLoop(ctx)
	if err := adminrpc.Serve(ctx, filepath.Join(*runtimeDir, "admin.sock"), manager); err != nil {
		log.Fatal(err)
	}
}
