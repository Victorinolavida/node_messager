BINARY := node_messager
CMD    := ./cmd
GO     := go

# correr en modo dev local (4 nodos en un proceso)
.PHONY: run-dev
run-dev:
	cp nodes-ejemplo.json nodes.json && $(GO) run $(CMD)

# correr con nodes.json actual (modo VM)
.PHONY: run
run:
	$(GO) run $(CMD)

# compilar binario Linux amd64 para desplegar en VMs con setup-vms.sh
.PHONY: build-linux
build-linux:
	GOOS=linux GOARCH=amd64 $(GO) build -ldflags "-X main.debug=false" -o $(BINARY)_linux_amd64 $(CMD)

.PHONY: test
test:
	$(GO) test ./...

.PHONY: tidy
tidy:
	$(GO) mod tidy

.PHONY: clean
clean:
	rm -f $(BINARY)_linux_amd64
	rm -rf data/ tickets/
