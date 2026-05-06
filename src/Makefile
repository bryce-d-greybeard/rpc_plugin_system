BINDIR := .tmp-bin

.PHONY: all build daemon ctl echo failure clean test

all: build

build: daemon ctl echo failure

daemon:
	mkdir -p $(BINDIR)
	go build -buildvcs=false -o $(BINDIR)/rpcplugind ./kernel-runtime-supervision/daemon-entrypoint/rpcplugind

ctl:
	mkdir -p $(BINDIR)
	go build -buildvcs=false -o $(BINDIR)/rpcpluginctl ./control-plane-ops/cli-status-control/rpcpluginctl

echo:
	mkdir -p $(BINDIR)
	go build -buildvcs=false -o $(BINDIR)/rpcplugin-echo ./plugin-authoring-sdk/echo-example/rpcplugin-echo

failure:
	mkdir -p $(BINDIR)
	go build -buildvcs=false -o $(BINDIR)/rpcplugin-failure ./plugin-authoring-sdk/failure-example/rpcplugin-failure

test:
	go test ./...

clean:
	rm -rf $(BINDIR)
