BINDIR := .tmp-bin

.PHONY: all build daemon ctl echo failure clean test coverage

all: build

build: daemon ctl echo failure

daemon:
	mkdir -p $(BINDIR)
	go build -buildvcs=false -o $(BINDIR)/rpcplugind ./kernel-runtime-supervision/daemon-entrypoint/rpcplugind
	chmod go-w $(BINDIR)/rpcplugind

ctl:
	mkdir -p $(BINDIR)
	go build -buildvcs=false -o $(BINDIR)/rpcpluginctl ./control-plane-ops/cli-status-control/rpcpluginctl
	chmod go-w $(BINDIR)/rpcpluginctl

echo:
	mkdir -p $(BINDIR)
	go build -buildvcs=false -o $(BINDIR)/rpcplugin-echo ./plugin-authoring-sdk/echo-example/rpcplugin-echo
	chmod go-w $(BINDIR)/rpcplugin-echo

failure:
	mkdir -p $(BINDIR)
	go build -buildvcs=false -o $(BINDIR)/rpcplugin-failure ./plugin-authoring-sdk/failure-example/rpcplugin-failure
	chmod go-w $(BINDIR)/rpcplugin-failure

test:
	go test ./...

coverage:
	go test -cover ./...

clean:
	rm -rf $(BINDIR)
