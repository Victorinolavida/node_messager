# Sistema Distribuido de Tickets de Soporte

Sistemas Distribuidos — Grupo 5, 26-2 — Segundo Entregable

## Descripción

Sistema de tickets de soporte técnico distribuido en 3 nodos (sucursales). Cada nodo tiene su propia base de datos SQLite con fragmento de los datos. Los nodos se comunican vía TCP con:

- **Consenso por quórum** — toda escritura requiere mayoría de votos antes de persistirse
- **Exclusión mutua centralizada** — un solo ingeniero puede ser asignado a la vez
- **Algoritmo Bully** — elección automática de nuevo maestro si el maestro falla
- **Heartbeat** — detección de nodos caídos y redistribución de tickets

---

## Requisitos

- Go 1.21 o superior
- No requiere CGO (SQLite puro en Go)

Verificar instalación:
```bash
go version
```

---

## Clonar y compilar

```bash
git clone <repo>
cd websockets_go

# Instalar dependencias
go mod tidy

# Compilar para Mac/Linux nativo
go build -o node_messager ./cmd

# Compilar para Linux amd64 (VMs)
make build-linux
```

---

## Ejecutar en modo local (pruebas — 1 sola máquina)

Todos los nodos corren en el mismo proceso. Ideal para probar todas las funciones sin VMs.

```bash
# Usar configuración de desarrollo (3 nodos en localhost)
cp nodes-dev.json nodes.json

go run ./cmd
# ó
./node_messager
```

El archivo `nodes-dev.json` tiene este contenido:
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

Al iniciar se verifica e inicializa automáticamente el esquema de la base de datos:
```
[sucursal1] db schema version: 1
[sucursal2] db schema version: 1
[sucursal3] db schema version: 1
```

---

## Ejecutar en VMs (modo producción — 1 proceso por VM)

### Paso 1 — Compilar el binario para Linux

```bash
make build-linux
# genera: node_messager_linux_amd64
```

### Paso 2 — Copiar el binario a cada VM

```bash
scp node_messager_linux_amd64 usuario@192.168.x.10:~/
scp node_messager_linux_amd64 usuario@192.168.x.11:~/
scp node_messager_linux_amd64 usuario@192.168.x.12:~/
```

### Paso 3 — Crear `nodes.json` en cada VM

Todos los campos `nodes` son iguales en todas las VMs. Solo cambia el campo `host`.

**VM 1 (sucursal1 — nodo maestro):**
```json
{
  "master_id": 1,
  "nodes": [
    { "id": 1, "name": "sucursal1", "host": "192.168.x.10", "port": 5001 },
    { "id": 2, "name": "sucursal2", "host": "192.168.x.11", "port": 5001 },
    { "id": 3, "name": "sucursal3", "host": "192.168.x.12", "port": 5001 }
  ],
  "host": { "id": 1, "name": "sucursal1", "host": "0.0.0.0", "port": 5001 }
}
```

**VM 2 (sucursal2):**
```json
{
  "master_id": 1,
  "nodes": [
    { "id": 1, "name": "sucursal1", "host": "192.168.x.10", "port": 5001 },
    { "id": 2, "name": "sucursal2", "host": "192.168.x.11", "port": 5001 },
    { "id": 3, "name": "sucursal3", "host": "192.168.x.12", "port": 5001 }
  ],
  "host": { "id": 2, "name": "sucursal2", "host": "0.0.0.0", "port": 5001 }
}
```

**VM 3 (sucursal3):**
```json
{
  "master_id": 1,
  "nodes": [
    { "id": 1, "name": "sucursal1", "host": "192.168.x.10", "port": 5001 },
    { "id": 2, "name": "sucursal2", "host": "192.168.x.11", "port": 5001 },
    { "id": 3, "name": "sucursal3", "host": "192.168.x.12", "port": 5001 }
  ],
  "host": { "id": 3, "name": "sucursal3", "host": "0.0.0.0", "port": 5001 }
}
```

### Paso 4 — Ejecutar en cada VM

```bash
chmod +x node_messager_linux_amd64

# Crear directorios necesarios
mkdir -p data logs messages tickets

# Ejecutar (recomendado iniciar VM1 primero)
./node_messager_linux_amd64
```

> **Nota:** Iniciar el nodo maestro (VM1) antes que los demás para evitar timeouts de heartbeat al arrancar.

---

## Menú de la aplicación

Al iniciar, aparece el menú interactivo:

```
  node messager — distributed ticket system

  ── mensajería ──
  1) send message          Enviar mensaje directo entre nodos
  2) broadcast             Difundir mensaje a todos los nodos
  3) messages per node     Ver mensajes enviados/recibidos
  4) logs per node         Ver logs del servidor TCP
  5) list nodes            Listar nodos configurados

  ── sistema de tickets ──
  7) raise ticket          Levantar ticket (exclusión mutua + consenso)
  8) close ticket          Cerrar ticket (ingeniero)
  9) list tickets          Ver todos los tickets (distribuido)
 10) add user              Agregar usuario a esta sucursal
 11) add engineer          Agregar ingeniero a esta sucursal
 12) add device            Agregar dispositivo (maestro distribuye equitativamente)
 13) list all users        Ver todos los usuarios (todas las sucursales)
 14) list all engineers    Ver todos los ingenieros (todas las sucursales)
 15) list all devices      Ver todos los dispositivos (todas las sucursales)

  6) quit
```

---

## Flujo de uso típico

### 1. Agregar ingenieros y usuarios

En cualquier sucursal:
```
> 11        # add engineer
  nombre: Juan Pérez
  ✓ ingeniero added

> 10        # add user
  nombre: Ana García
  ✓ usuario added
```

### 2. Agregar un dispositivo

El maestro lo distribuye a la sucursal con menos dispositivos:
```
> 12        # add device
  nombre: Laptop-001
  tipo: Laptop
  ✓ device queued for distribution
```

### 3. Levantar un ticket

El sistema adquiere exclusión mutua, busca ingeniero disponible, aplica consenso:
```
> 7         # raise ticket
  usuario ID: 123456789
  dispositivo ID: 987654321

  raising ticket (acquiring lock + consensus)...
  ✓ ticket raised
```

Se genera automáticamente el folio en `tickets/IDUSUARIO-IDINGENIERO-IDSUCURSAL-IDTICKET.txt`.

### 4. Cerrar un ticket

```
> 8         # close ticket
  ticket ID: 123
  ingeniero ID: 456
  ✓ ticket closed
```

### 5. Ver todos los tickets

```
> 9         # list tickets
  ID          USUARIO     INGENIERO   SUCURSAL    DISPOSITIVO  ESTADO    FOLIO
  123456789   111         222         1           333          ABIERTO   111-222-1-123456789.txt
```

---

## Estructura de archivos en tiempo de ejecución

```
data/
  sucursal1.db      ← SQLite, fragmento del nodo 1 (schema v1)
  sucursal2.db      ← SQLite, fragmento del nodo 2
  sucursal3.db      ← SQLite, fragmento del nodo 3
logs/
  sucursal1.log     ← log del servidor TCP del nodo 1
messages/
  sucursal1.jsonl   ← historial de mensajes enviados/recibidos
tickets/
  111-222-1-123.txt ← folios generados (USUARIO-INGENIERO-SUCURSAL-TICKET)
```

---

## Arquitectura distribuida

```
sucursal1 (maestro)      sucursal2               sucursal3
├── TCP :5001            ├── TCP :5001            ├── TCP :5001
├── SQLite (s=1)         ├── SQLite (s=2)         ├── SQLite (s=3)
├── Consensus Engine     ├── Consensus Engine     ├── Consensus Engine
├── Mutex Engine (lock)  ├── Mutex Engine         ├── Mutex Engine
├── Election Engine      ├── Election Engine      ├── Election Engine
└── Heartbeat Monitor    └── Heartbeat Monitor    └── Heartbeat Monitor
```

**Fragmentación:** cada nodo almacena solo los registros donde `sucursal_id = su_id`. Para ver todos los datos se hace una consulta broadcast a todos los nodos.

**Si el maestro cae:** el nodo con mayor ID entre los activos se declara nuevo maestro (Bully). Los tickets abiertos del nodo caído se redistribuyen a ingenieros disponibles.

---

## Agregar un cuarto nodo

1. En `nodes.json` (y `nodes-dev.json`), agregar:
   ```json
   { "id": 4, "name": "sucursal4", "host": "192.168.x.13", "port": 5001 }
   ```
2. Cambiar `master_id` si se desea otro maestro.
3. Compilar y desplegar igual que las otras VMs.

---

## Comandos útiles

```bash
make build           # compilar para la máquina actual
make build-linux     # compilar para Linux amd64
make test            # correr tests
make clean           # limpiar binarios y datos locales
go mod tidy          # actualizar dependencias
```
