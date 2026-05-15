CMD := ./cmd
GO  := go

# correr en modo dev local (4 nodos en un proceso)
.PHONY: run-dev
run-dev:
	cp nodes-ejemplo.json nodes.json && $(GO) run $(CMD)

# correr con nodes.json actual (modo VM)
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
	rm -rf data/
