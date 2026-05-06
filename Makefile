BINARY     := node_messager
CMD        := ./cmd
GO         := go

# Host build (macOS/native)
.PHONY: build
build:
	$(GO) build -o $(BINARY) $(CMD)

.PHONY: build-prod
build-prod:
	$(GO) build -ldflags "-X main.debug=false" -o $(BINARY) $(CMD)

# Linux/Debian cross-compile (amd64)
.PHONY: build-linux
build-linux:
	GOOS=linux GOARCH=amd64 $(GO) build -ldflags "-X main.debug=false" -o $(BINARY)_linux_amd64 $(CMD)

# Linux/Debian cross-compile (arm64 — Raspberry Pi / ARM servers)
.PHONY: build-linux-arm
build-linux-arm:
	GOOS=linux GOARCH=arm64 $(GO) build -ldflags "-X main.debug=false" -o $(BINARY)_linux_arm64 $(CMD)

# Run with all 3 nodes locally (dev/testing)
.PHONY: run-dev
run-dev:
	cp nodes-dev.json nodes.json && $(GO) run $(CMD)

# Run with original nodes.json (VM/host mode)
.PHONY: run
run:
	$(GO) run $(CMD)

.PHONY: test
test:
	$(GO) test ./...

.PHONY: tidy
tidy:
	$(GO) mod tidy

.PHONY: clean
clean:
	rm -f $(BINARY) $(BINARY)_linux_amd64 $(BINARY)_linux_arm64
	rm -rf data/ tickets/
