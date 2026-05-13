# node_messager — Sistema Distribuido de Tickets de Soporte

Sistema TCP distribuido de 3–4 sucursales con consenso, exclusión mutua y elección de líder.

## Requisitos

- Go 1.21+
- No requiere CGO (SQLite puro en Go)

## Compilar

```bash
go mod tidy
go build -o node_messager ./cmd           # nativo
make build-linux                           # Linux amd64 para VMs
```

## nodes.json

**Modo local (pruebas — todos los nodos en 1 proceso):**
```json
{
  "master_id": 1,
  "nodes": [
    { "id": 1, "name": "sucursal1", "host": "localhost", "port": 5001 },
    { "id": 2, "name": "sucursal2", "host": "localhost", "port": 5002 },
    { "id": 3, "name": "sucursal3", "host": "localhost", "port": 5003 }
  ]
}
```

**Modo VM (1 proceso por máquina) — solo cambia `host_id` en cada VM:**
```json
{
  "master_id": 1,
  "host_id": 2,
  "nodes": [
    { "id": 1, "name": "sucursal1", "host": "192.168.100.10", "port": 5001 },
    { "id": 2, "name": "sucursal2", "host": "192.168.100.11", "port": 5001 },
    { "id": 3, "name": "sucursal3", "host": "192.168.100.12", "port": 5001 }
  ]
}
```

| Campo | Descripción |
|-------|------------|
| `master_id` | ID de la sucursal que arranca como maestro |
| `host_id` | ID de la sucursal que corre en esta máquina (omitir en modo local) |
| `nodes` | Lista de todas las sucursales con sus IPs y puertos |

> **Regla:** `host_id` debe coincidir con un `id` en el array `nodes`. Si no coincide, la app falla al arrancar con error claro.

## Ejecutar

```bash
# modo local
cp nodes-ejemplo.json nodes.json && go run ./cmd

# modo VM (después de copiar el binario y nodes.json correcto)
./node_messager_linux_amd64
```

## Despliegue rápido con script

```bash
./setup.sh <IP1> <IP2> <IP3> <IP4> [usuario_ssh]

# solo generar configs (sin desplegar)
./setup.sh 192.168.100.10 192.168.100.11 192.168.100.12 192.168.100.13 --only-configs
```

## Archivos en runtime

```
data/<sucursal>.db       SQLite por nodo (gitignored)
logs/<sucursal>.log      Logs TCP (gitignored)
messages/<sucursal>.jsonl Historial mensajes (gitignored)
tickets/<folio>.txt      Folios de tickets (gitignored)
```

## Tests

```bash
go test ./...
```
