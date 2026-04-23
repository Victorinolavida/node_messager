# node_messager

Terminal app that runs multiple HTTP/WebSocket servers and lets you send messages between them via a live TUI.

## What it does

- Starts 4 named nodes (alpha, beta, gamma, delta), each with an HTTP server and a `/ws` WebSocket endpoint
- Split TUI: left panel = menu, right panel = live logs from all servers
- Send a direct message from one node to another, or broadcast from one node to all
- Logs every received message and ack timestamp in the log panel

## Requirements

- Go 1.22+
- Ports 5000–5003 free on 127.0.0.1

## Run

```bash
git clone https://github.com/Victorinolavida/node_messager.git
cd node_messager
go run ./cmd
```

## Usage

Navigate the menu with arrow keys or `j`/`k`. Press `enter` to select, `esc` to go back, `q` to quit.

| Option | Flow |
|---|---|
| **Send a message** | Select FROM node → select TO node (self excluded) → type message → `enter` |
| **Broadcast a message** | Select FROM node → type message → `enter` (sends to all nodes) |
| **List all nodes** | Shows all nodes with host, port, and WebSocket URL |

Messages appear in the log panel in real time, prefixed with the receiving node name.

## Project structure

```
cmd/            entry point — defines nodes, starts servers, launches TUI
pkg/
  node/         Node struct {id, name, host, port}
  http_server/  HTTP server per node with /ws route
  hub/          WebSocket hub — manages clients, broadcasts, logs msg + ack
  wsclient/     WebSocket client used by TUI to dial and send messages
  logbuffer/    Thread-safe ring buffer (500 lines) shared between servers and TUI
  logger/       Zap-based logger, supports writing to logbuffer
internal/
  adapters/tui/ Bubble Tea TUI — split layout, multi-step send/broadcast flow
```

## WebSocket

Each node exposes a WebSocket endpoint at `ws://127.0.0.1:<port>/ws`.

Connect any WebSocket client to receive broadcast messages:

```
ws://127.0.0.1:5000/ws  ← alpha
ws://127.0.0.1:5001/ws  ← beta
ws://127.0.0.1:5002/ws  ← gamma
ws://127.0.0.1:5003/ws  ← delta
```
