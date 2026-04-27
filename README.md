# node_messager

Terminal app that runs HTTP/WebSocket servers on named nodes and lets you send messages between them via a live TUI.

## What it does

- Loads nodes from `nodes.json` — supports any number of nodes on any hosts/ports
- Optional `host` key (or `self_hosted: true` flag on a node) marks which node is the local machine
- When a host node is identified: only that node's server runs locally; FROM is pre-selected in the TUI
- Split TUI: left panel (1/4) = menu/actions, right panel (3/4) = live server logs
- Send a direct message or broadcast from one node to all others
- Persists messages to `messages/<name>.jsonl` (one JSON-Lines file per node, both sent and received)
- Persists server logs to `logs/<name>.log`

## Requirements

- Go 1.22+
- `nodes.json` in the working directory

## nodes.json format

All nodes on one machine (local dev):

```json
{
  "nodes": [
    { "id": 0, "name": "alpha", "host": "127.0.0.1", "port": 5000 },
    { "id": 1, "name": "beta",  "host": "127.0.0.1", "port": 5001 }
  ]
}
```

Two physical machines — mark the local node with `self_hosted: true`:

```json
{
  "nodes": [
    { "id": 0, "name": "mac",   "host": "192.168.1.10", "port": 5000, "self_hosted": true },
    { "id": 1, "name": "linux", "host": "192.168.1.20", "port": 5000 }
  ]
}
```

Or use the explicit `host` key:

```json
{
  "nodes": [
    { "id": 1, "name": "linux", "host": "192.168.1.20", "port": 5000 }
  ],
  "host": { "id": 0, "name": "mac", "host": "192.168.1.10", "port": 5000 }
}
```

If neither `self_hosted` nor `host` is set, the app auto-detects the host node by matching the machine's outbound IP against the nodes list.

## Run

```bash
git clone https://github.com/Victorinolavida/node_messager.git
cd node_messager
# edit nodes.json to match your setup
go run ./cmd
```

## Build

```bash
# native (dev, debug logs)
make build

# native (production, info logs only)
make build-prod

# Linux/Debian amd64 cross-compile
make build-linux

# Linux/Debian arm64 cross-compile (Raspberry Pi / ARM servers)
make build-linux-arm

# clean all binaries
make clean
```

Or manually:

```bash
# development
go build -o node_messager ./cmd

# production
go build -ldflags "-X main.debug=false" -o node_messager ./cmd

# cross-compile for Debian/Linux amd64
GOOS=linux GOARCH=amd64 go build -ldflags "-X main.debug=false" -o node_messager_linux_amd64 ./cmd
```

## Deploy to Linux

```bash
make build-linux
scp node_messager_linux_amd64 user@192.168.1.20:~/node_messager
scp nodes.json user@192.168.1.20:~/nodes.json
# on the remote machine:
# ./node_messager
```

## Usage

Navigate with arrow keys or `j`/`k`. `enter` to select, `esc` to go back, `q` to quit.

| Option | Flow |
|---|---|
| **Send a message** | (select FROM if no host set) → select TO node → type message → `enter` |
| **Broadcast** | (select FROM if no host set) → type message → `enter` (sends to all nodes) |
| **View node logs** | (select node if no host set) → shows last 50 messages with timestamp and type |
| **List all nodes** | Shows all nodes with host, port, and WebSocket URL |

## Message format (wire)

```json
{
  "id":        "uuid-v4",
  "type":      "MSG | BROADCAST",
  "from_node": "mac",
  "to_node":   "linux",
  "content":   "hello",
  "send_at":   "2024-01-01T00:00:00Z"
}
```

## Persisted files

| Path | Content |
|---|---|
| `logs/<name>.log` | Plain-text server logs (created automatically, git-ignored) |
| `messages/<name>.jsonl` | JSON-Lines message history — both sent and received (git-ignored) |

## Message entry format

```json
{"at":"2024-01-01T00:00:00Z","type":"sent","msg":{...}}
{"at":"2024-01-01T00:00:00Z","type":"received","msg":{...}}
```

## Log levels

| Level | Logs shown |
|---|---|
| **debug** (default `go run`) | startup, received messages, connect/disconnect, ack timestamps |
| **info** (`-X main.debug=false`) | startup and received messages only |

## Project structure

```
cmd/                  entry point — config load, server startup, TUI launch
nodes.json            node definitions (not committed — edit per machine)
pkg/
  node/               Node struct + GetLocalIP()
  http_server/        HTTP server per node with /ws route
  hub/                WebSocket hub — registry, broadcast loop, message storage
  wsclient/           Dial-and-send WebSocket client (used by TUI)
  logbuffer/          Thread-safe ring buffer (500 lines) shared between servers and TUI
  msgstore/           Per-node message store — ring buffer + JSON-Lines file
  dto/                Wire message struct with JSON tags
  logger/             Zap logger — fans out to TUI buffer (color) and log file (plain)
internal/
  config/             LoadConfig — parses nodes.json, resolves host node
  entities/           Domain types: Message, MessageType, NodeName, Timestamp
  ports/              Primary/secondary port interfaces
  usecases/           MessengerUseCase
  adapters/
    tui/              Bubble Tea TUI — split layout, multi-step state machine
    websocket/        WebSocket adapter stub
```
