# node_messager

Terminal app that runs multiple HTTP/WebSocket servers and lets you send messages between them via a live TUI.

## What it does

- Starts 4 named nodes (alpha, beta, gamma, delta), each with an HTTP server and a `/ws` WebSocket endpoint
- Split TUI: left panel (1/4) = menu, right panel (3/4) = live logs from all servers
- Send a direct message from one node to another, or broadcast from one node to all
- Logs every received message in the TUI panel and to a per-node log file (`logs/<name>.log`)
- Messages are stored in a per-node in-memory ring buffer (last 50); viewable from the TUI

## Requirements

- Go 1.22+
- Ports 5000–5003 free on 127.0.0.1

## Run

```bash
git clone https://github.com/Victorinolavida/node_messager.git
cd node_messager
go run ./cmd
```

## Build

```bash
# development — debug level (connect/disconnect + ack logs visible)
go build -o app ./cmd

# production — info level only (startup + received messages)
go build -ldflags "-X main.debug=false" -o app ./cmd
```

## Log levels

| Level | Visible logs |
|---|---|
| **debug** (default `go run`) | startup, received messages, client connect/disconnect, ack timestamps |
| **info** (`-X main.debug=false`) | startup and received messages only |

## Usage

Navigate the menu with arrow keys or `j`/`k`. Press `enter` to select, `esc` to go back, `q` to quit.

| Option | Flow |
|---|---|
| **Send a message** | Select FROM node → select TO node (self excluded) → type message → `enter` |
| **Broadcast a message** | Select FROM node → type message → `enter` (sends to all nodes) |
| **View node logs** | Select node → shows last 50 received messages with timestamp and type |
| **List all nodes** | Shows all nodes with host, port, and WebSocket URL |

Messages appear in the log panel in real time. Validation: empty messages blocked, self-send excluded from TO list.

## Message format (wire)

Messages are sent as JSON over WebSocket:

```json
{
  "id":         "uuid-v4",
  "type":       "MSG | BROADCAST",
  "from_node":  "alpha",
  "to_node":    "beta",
  "content":    "hello",
  "created_at": "2024-01-01T00:00:00Z"
}
```

## Log files

Each node writes plain-text logs to `logs/<name>.log` (created automatically, git-ignored).

## Project structure

```
cmd/                  entry point — node definitions, server startup, TUI launch
pkg/
  node/               Node struct {id, name, host, port}
  http_server/        HTTP server per node with /ws route auto-registered
  hub/                WebSocket hub — client registry, broadcast loop, msg/ack logging
  wsclient/           Dial-and-send WebSocket client used by TUI
  logbuffer/          Thread-safe ring buffer (500 lines) shared between servers and TUI
  msgstore/           Per-node in-memory message store (ring buffer, last 50 entries)
  dto/                Wire message struct with JSON tags
  logger/             Zap logger — NewLoggerForNode fans out to TUI buffer (color) + file (plain)
internal/
  entities/           Domain types: Message, MessageType, NodeName, Timestamp
  dto/                Internal DTO mirror
  ports/
    primary/          MessengerPort interface
    secondary/        WebsocketPort, StoragePort interfaces
  usecases/           MessengerUseCase
  adapters/
    tui/              Bubble Tea TUI — split layout, multi-step state machine
    websocket/        WebSocket adapter stub
```

## WebSocket

Each node exposes a WebSocket endpoint at `ws://127.0.0.1:<port>/ws`.

```
ws://127.0.0.1:5000/ws  ← alpha
ws://127.0.0.1:5001/ws  ← beta
ws://127.0.0.1:5002/ws  ← gamma
ws://127.0.0.1:5003/ws  ← delta
```

Connect any external WebSocket client to a node to receive all messages broadcast through its hub.
