BINDIR := .tmp-bin
ARTIFACTS_DIR := ../workflow/artifacts
COVERAGE_PROFILE := $(ARTIFACTS_DIR)/coverage.out
COVERAGE_SUMMARY := $(ARTIFACTS_DIR)/coverage-summary.txt

.PHONY: all build daemon ctl echo failure clean test coverage

all: build

build: daemon ctl echo failure

daemon:
	mkdir -p $(BINDIR)
	go build -buildvcs=false -o $(BINDIR)/rpcplugind ./cmd/rpcplugind
	chmod go-w $(BINDIR)/rpcplugind

ctl:
	mkdir -p $(BINDIR)
	go build -buildvcs=false -o $(BINDIR)/rpcpluginctl ./cmd/rpcpluginctl
	chmod go-w $(BINDIR)/rpcpluginctl

echo:
	mkdir -p $(BINDIR)
	go build -buildvcs=false -o $(BINDIR)/rpcplugin-echo ./examples/rpcplugin-echo
	chmod go-w $(BINDIR)/rpcplugin-echo

failure:
	mkdir -p $(BINDIR)
	go build -buildvcs=false -o $(BINDIR)/rpcplugin-failure ./examples/rpcplugin-failure
	chmod go-w $(BINDIR)/rpcplugin-failure

test:
	go test ./...

coverage:
	mkdir -p $(ARTIFACTS_DIR)
	go test -coverprofile=$(COVERAGE_PROFILE).tmp ./... > $(COVERAGE_SUMMARY).tmp
	{ head -n 1 $(COVERAGE_PROFILE).tmp; tail -n +2 $(COVERAGE_PROFILE).tmp | LC_ALL=C sort; } > $(COVERAGE_PROFILE).sorted
	mv $(COVERAGE_PROFILE).sorted $(COVERAGE_PROFILE).tmp
	go tool cover -func=$(COVERAGE_PROFILE).tmp >> $(COVERAGE_SUMMARY).tmp
	mv $(COVERAGE_PROFILE).tmp $(COVERAGE_PROFILE)
	mv $(COVERAGE_SUMMARY).tmp $(COVERAGE_SUMMARY)
	cat $(COVERAGE_SUMMARY)

clean:
	rm -rf $(BINDIR)
