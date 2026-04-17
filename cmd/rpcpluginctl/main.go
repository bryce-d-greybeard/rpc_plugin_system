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
	"rpc_plugin_system/internal/kernel"
)

func main() {
	var runtimeDir string
	flag.StringVar(&runtimeDir, "runtime-dir", filepath.Join(os.TempDir(), "rpc_plugin_system"), "runtime directory")
	flag.Parse()

	if flag.NArg() != 1 {
		log.Fatalf("usage: %s [-runtime-dir DIR] <status|restart>", os.Args[0])
	}

	client, err := adminrpc.Dial(filepath.Join(runtimeDir, "admin.sock"), 2*time.Second)
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
	case "help", "-h", "--help":
		fmt.Printf("usage: %s [-runtime-dir DIR] <status|restart>\n", os.Args[0])
		return
	default:
		log.Fatalf("unknown command %q, want status or restart", flag.Arg(0))
	}
	if err != nil {
		log.Fatal(err)
	}
	if err := cli.WriteState(os.Stdout, state); err != nil {
		log.Fatal(err)
	}
}
