# node_messager — codebase overview

## What it does

TCP-based node messaging system. Nodes connect to each other's TCP servers, send direct messages or broadcasts. All sent/received messages are persisted to disk per node.

## Run modes

**Dev mode** — no `host` key in `nodes.json`. All nodes run locally. Every node starts a TCP server and gets a file-backed message store. CLI lets you pick any node as sender.

**Host mode** — `nodes.json` has a `host` entry. Only the host node's server runs locally. Remote nodes get in-memory-only stores (they run on other machines). CLI uses the host node automatically as sender; messages/logs views show host node's data only.

## Package layout

```
cmd/main.go                     — entry point: load config, start servers, run CLI
internal/
  adapters/
    cli/cli.go                  — interactive plain-text CLI (stdin loop)
    tui/tui.go                  — Bubbletea TUI (unused in current binary)
internal/config/config.go       — load nodes.json via koanf
pkg/
  dto/message.go                — Message struct (id, type, from_node, to_node, content, send_at)
  hub/hub.go                    — WebSocket-style hub: fan-out, saves Received to store
  msgstore/store.go             — thread-safe in-memory + JSONL file store
  node/node.go                  — Node struct (id, name, host, port)
  tcp_server/server.go          — TCP listener → hub
  tcpclient/client.go           — TCP client with Recv channel
  logbuffer/buffer.go           — ring buffer for log lines (used by TUI)
  logger/logger.go              — zap logger helpers
```

## Message flow

```
CLI sendMsg / broadcast
  └─ connPool.get(to)           — reuse or create tcpclient.Client
  └─ client.Send(json)          — writes JSON line to TCP conn
  └─ stores[from.ID].Save(Sent) — sender store updated synchronously

TCP server (hub)
  └─ readPump scans line
  └─ hub.broadcast channel
  └─ hub.Run saves Received     — receiver store updated in hub goroutine
  └─ writePump fans-out to all connected clients (echo included)
```

## Persistence

- `messages/<name>.jsonl` — one JSON entry per line, loaded on restart
- `logs/<name>.log` — zap logs for that node's server
- In host mode only the host node's files are written locally

## Key invariants

- **Sent** entries written by CLI immediately after `c.Send` succeeds
- **Received** entries written exclusively by the hub (`hub.Run`) — never by the CLI
- Broadcast excludes the sender node from targets (no self-send)
- In host mode, messages/logs views show only the host node's data

## Config format (`nodes.json`)

```json
{ "nodes": [...], "host": { "id": 1, "name": "mynode", "host": "0.0.0.0", "port": 5010 } }
```

`host` is optional. When present, only that node's server starts locally.

## Tests

| Package | What's covered |
|---------|---------------|
| `pkg/msgstore` | Save/Latest, file persistence, restart reload, max cap |
| `pkg/hub` | Received save, delivery to other clients, echo |
| `pkg/tcp_server` | Port conflict, multi-client, context cancel, broadcast |
| `internal/adapters/cli` | sendMsg/broadcast save Sent+Received, no self-echo, formatEntries |
| `internal/adapters/tui` | formatEntries output |

## Build

```bash
go build ./cmd            # dev binary
go build -ldflags "-X main.debug=false" ./cmd   # prod (quieter logs)
go test ./...
```
