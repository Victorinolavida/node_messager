# Sistema Distribuido de Tickets de Soporte

Sistemas Distribuidos — Grupo 5, 26-2 — Segundo Entregable

## Descripción

Sistema de tickets de soporte técnico distribuido en 3–4 sucursales. Cada nodo tiene su propia base de datos SQLite con fragmento de los datos. Los nodos se comunican vía TCP con:

- **Consenso por quórum** — toda escritura requiere mayoría de votos antes de persistirse
- **Exclusión mutua centralizada** — un solo ingeniero puede ser asignado a la vez
- **Algoritmo Bully (inverso)** — elección automática de nuevo maestro si el maestro falla
- **Heartbeat** — detección de nodos caídos y redistribución de tickets
- **Reintentos automáticos** — 3 intentos con log en cada fallo

---

## Requisitos

- Go 1.21 o superior
- No requiere CGO (SQLite puro en Go)

```bash
go version
```

---

## Clonar y ejecutar

```bash
git clone <repo>
cd node_messager
go mod tidy
go run ./cmd
```

---

## Formato de nodes.json

### Campos

| Campo | Tipo | Descripción |
|-------|------|-------------|
| `master_id` | int | ID de la sucursal que arranca como maestro inicial |
| `host_id` | int | ID de la sucursal que corre en esta máquina. **Omitir en modo local** |
| `nodes` | array | Lista de **todas** las sucursales con IP y puerto |

> **Importante:** `host_id` debe existir en el array `nodes`. Si no coincide, la app falla al arrancar con error descriptivo.

### Modo local (sin `host_id`)

Todos los nodos corren en el mismo proceso. No se necesita `host_id`.

```json
{
  "master_id": 1,
  "nodes": [
    { "id": 1, "name": "sucursal1", "host": "localhost", "port": 5001 },
    { "id": 2, "name": "sucursal2", "host": "localhost", "port": 5002 },
    { "id": 3, "name": "sucursal3", "host": "localhost", "port": 5003 },
    { "id": 4, "name": "sucursal4", "host": "localhost", "port": 5004 }
  ]
}
```

### Modo VM (con `host_id`)

El array `nodes` es **idéntico en todas las VMs**. Solo cambia `host_id` según qué máquina es.

**VM 1 — sucursal1 (maestro):**
```json
{
  "master_id": 1,
  "host_id": 1,
  "nodes": [
    { "id": 1, "name": "sucursal1", "host": "192.168.100.102", "port": 5001 },
    { "id": 2, "name": "sucursal2", "host": "192.168.100.103", "port": 5001 },
    { "id": 3, "name": "sucursal3", "host": "192.168.100.104", "port": 5001 },
    { "id": 4, "name": "sucursal4", "host": "192.168.100.105", "port": 5001 }
  ]
}
```

**VM 2 — sucursal2:** igual que arriba pero `"host_id": 2`

**VM 3 — sucursal3:** igual que arriba pero `"host_id": 3`

**VM 4 — sucursal4:** igual que arriba pero `"host_id": 4`

---

## Ejecutar en modo local (pruebas)

```bash
cp nodes-ejemplo.json nodes.json
go run ./cmd
# ó
./node_messager
```

Al iniciar se verifica el esquema de la base de datos:
```
[sucursal1] db schema version: 2
[sucursal2] db schema version: 2
[sucursal3] db schema version: 2
[sucursal4] db schema version: 2
```

---

## Despliegue en VMs

**En cada VM (iniciar VM1 primero):**

**Paso 1 — Clonar el repo:**
```bash
git clone <repo> ~/node_messager
cd ~/node_messager
go mod tidy
```

**Paso 2 — Correr setup (activa NTP y crea nodes.json):**
```bash
bash setup.sh <host_id>
# ejemplo en VM2:
bash setup.sh 2
```

**Paso 3 — Iniciar el nodo:**
```bash
cd ~/node_messager
go run ./cmd
```

---

## Menú de la aplicación

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
  8) close ticket          Cerrar ticket
  9) list tickets          Ver todos los tickets (distribuido)
 10) add user              Agregar usuario a esta sucursal
 11) add engineer          Agregar ingeniero a esta sucursal
 12) add device            Agregar dispositivo (maestro distribuye por ingeniero)
 13) list all users        Ver todos los usuarios (todas las sucursales)
 14) list all engineers    Ver todos los ingenieros (todas las sucursales)
 15) list all devices      Ver todos los dispositivos (todas las sucursales)

  6) quit
```

---

## Flujo de uso típico

### 1. Agregar ingenieros y usuarios
```
> 11  →  nombre: Juan Pérez   →  ✓ ingeniero added
> 10  →  nombre: Ana García   →  ✓ usuario added
```

### 2. Agregar dispositivo
El maestro lo asigna al ingeniero con menos dispositivos:
```
> 12  →  nombre: Laptop-001  →  tipo: Laptop  →  ✓ device queued for distribution
```

### 3. Levantar ticket
Adquiere lock → busca ingeniero disponible → consenso → genera folio:
```
> 7
  usuario ID: 10000000001
  dispositivo ID: 10000000001
  raising ticket (acquiring lock + consensus)...
  ✓ ticket raised
```
Folio generado: `IDUSUARIO-IDINGENIERO-IDSUCURSAL-IDTICKET` (almacenado en la BD)

### 4. Cerrar ticket
```
> 8  →  ticket ID: xxx  →  ingeniero ID: yyy  →  ✓ ticket closed
```

---

## Identificadores

Los IDs se generan como `nodeID × 10,000,000,000 + contador`. Esto garantiza que dos nodos distintos nunca generen el mismo ID.

```
sucursal1: 10000000001, 10000000002 ...
sucursal2: 20000000001, 20000000002 ...
```

---

## Si el maestro falla

Reverse Bully: el nodo con **menor ID activo** toma el rol de maestro automáticamente.

```
sucursal1 (maestro) cae
sucursal2 detecta → elección → sucursal2 gana (menor ID activo)
sucursal1 vuelve  → anuncia COORDINATOR → recupera el rol
```

---

## Estructura de archivos en runtime

```
data/
  sucursal1.db      ← SQLite por nodo (schema v2)
logs/
  sucursal1.log     ← logs del servidor TCP
messages/
  sucursal1.jsonl   ← historial de mensajes
```

Todos estos directorios son creados automáticamente y están en `.gitignore`.

---

## Estructura de nodos

El sistema opera con **4 sucursales**. El array `nodes` es idéntico en todas las VMs; solo cambia `host_id`. Para cambiar IPs, editar `nodes.json` en cada VM. No se necesita recompilar.

---

## Comandos

```bash
make run-dev         # modo local (4 nodos en un proceso)
make run             # modo VM (nodes.json ya configurado)
make test            # correr tests
make clean           # limpiar data/
go mod tidy          # actualizar dependencias
```
