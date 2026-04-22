BINDIR := .tmp-bin

.PHONY: all build daemon ctl echo failure clean test

all: build

build: daemon ctl echo failure

daemon:
	mkdir -p $(BINDIR)
	go build -buildvcs=false -o $(BINDIR)/rpcplugind ./cmd/rpcplugind

ctl:
	mkdir -p $(BINDIR)
	go build -buildvcs=false -o $(BINDIR)/rpcpluginctl ./cmd/rpcpluginctl

echo:
	mkdir -p $(BINDIR)
	go build -buildvcs=false -o $(BINDIR)/rpcplugin-echo ./cmd/rpcplugin-echo

failure:
	mkdir -p $(BINDIR)
	go build -buildvcs=false -o $(BINDIR)/rpcplugin-failure ./cmd/rpcplugin-failure

test:
	go test ./...

clean:
	rm -rf $(BINDIR)
