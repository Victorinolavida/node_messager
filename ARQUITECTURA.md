# Sistema Distribuido de Tickets de Soporte — Arquitectura

## Visión general

El sistema es una red de 3–4 nodos (sucursales) que se comunican entre sí usando TCP. Cada nodo tiene su propio servidor TCP, su propia base de datos SQLite local, y un conjunto de motores distribuidos que coordinan las operaciones entre nodos.

No existe un servidor central compartido. La "distribución" se implementa en la capa de comunicación: los nodos se hablan entre sí usando mensajes JSON por TCP para acordar cambios, asignar tareas y detectar fallas.

---

## Estructura de nodos

```
┌────────────────────────────────────┐   ┌────────────────────────────────────┐
│         sucursal1 (maestro)        │   │            sucursal2               │
│                                    │   │                                    │
│  TCP :5001  ←──────────────────────┼───┼──► TCP :5002                      │
│  SQLite: sucursal1.db              │   │  SQLite: sucursal2.db              │
│    INGENIEROS (sucursal_id=1)      │   │    INGENIEROS (sucursal_id=2)      │
│    USUARIOS   (sucursal_id=1)      │   │    USUARIOS   (sucursal_id=2)      │
│    DISPOSITIVOS (sucursal_id=1)    │   │    DISPOSITIVOS (sucursal_id=2)    │
│    TICKETS    (sucursal_id=1)      │   │    TICKETS    (sucursal_id=2)      │
│                                    │   │                                    │
│  Motor de Consenso                 │   │  Motor de Consenso                 │
│  Motor de Mutex (gestor del lock)  │   │  Motor de Mutex (solicitante)      │
│  Motor de Elección                 │   │  Motor de Elección                 │
│  Monitor de Heartbeat              │   │  Monitor de Heartbeat              │
└────────────────────────────────────┘   └────────────────────────────────────┘
```

### Fragmentación de datos

Cada nodo almacena **únicamente sus propios registros**, identificados por `sucursal_id`. El total de la base de datos existe como unión lógica de todos los fragmentos.

```
sucursal1.db               sucursal2.db               sucursal3.db
INGENIEROS: ing1, ing2     INGENIEROS: ing3, ing4     INGENIEROS: ing5
USUARIOS:   usr1, usr2     USUARIOS:   usr3           USUARIOS:   usr4, usr5
TICKETS:    tkt1           TICKETS:    tkt2, tkt3     TICKETS:    tkt4
```

Para consultar todos los ingenieros: se envía un mensaje `QUERY` a todos los nodos, cada uno responde con sus registros locales, y el nodo que preguntó los combina.

---

## Componentes internos de cada nodo

### 1. Servidor TCP (`pkg/tcp_server`)

Escucha conexiones entrantes en el puerto configurado. Por cada conexión acepta un cliente en el hub.

### 2. Hub (`pkg/hub`)

Gestiona todos los clientes TCP conectados. Cuando llega un mensaje:
1. Lo guarda en el store de mensajes (historial JSONL)
2. Llama al **Dispatcher** de forma asíncrona
3. Reenvía el mensaje a todos los clientes conectados (fan-out)

### 3. Dispatcher (`internal/dispatcher`)

Enruta cada mensaje entrante al motor correcto según su tipo (`msg.Type`):

| Tipo de mensaje | Motor destino |
|-----------------|--------------|
| `PING` / `PONG` | Heartbeat |
| `PROPOSE` / `VOTE_YES` / `VOTE_NO` / `COMMIT` | Consenso |
| `LOCK_REQUEST` / `LOCK_GRANT` / `LOCK_RELEASE` | Mutex |
| `ELECTION` / `ELECTION_OK` / `COORDINATOR` | Elección |
| `QUERY` / `QUERY_RESPONSE` | Servicio de tickets |
| `ADD_DEVICE` | Servicio de tickets (solo maestro) |
| `NODE_DEAD` | Redistribución de tickets |

### 4. Sender Pool (`pkg/sender`)

Pool de conexiones TCP salientes reutilizables. Evita abrir una nueva conexión TCP por cada mensaje.

---

## Protocolos distribuidos

### Consenso (quórum de mayoría)

Antes de cualquier escritura a la base de datos, el nodo que inicia la operación debe obtener la aprobación de la mayoría de los nodos activos.

```
Nodo iniciador                    Nodo 2                    Nodo 3
      │                               │                          │
      │──── PROPOSE (roundID, op) ───►│                          │
      │──── PROPOSE (roundID, op) ────────────────────────────►  │
      │                               │                          │
      │◄─── VOTE_YES (roundID) ───────│                          │
      │◄─── VOTE_YES (roundID) ──────────────────────────────────│
      │                               │                          │
      │  (mayoría alcanzada)          │                          │
      │                               │                          │
      │──── COMMIT (roundID, op) ────►│  aplica en SQLite        │
      │──── COMMIT (roundID, op) ─────────────────────────────►  │  aplica en SQLite
      │                               │                          │
      │  (aplica en SQLite local)     │                          │
```

- Quórum requerido: `(nodos_activos / 2) + 1`
- Timeout de votos: 3 segundos
- Si no hay quórum: la operación falla con error al usuario

### Exclusión mutua (lock centralizado)

El nodo maestro actúa como gestor del lock de asignación de ingenieros. Solo un nodo puede asignar un ticket a la vez.

```
Nodo 2 (quiere asignar)          Maestro (nodo 1)
        │                               │
        │──── LOCK_REQUEST ────────────►│
        │                               │ (lock libre)
        │◄─── LOCK_GRANT ───────────────│
        │                               │
        │  ... asigna ticket ...        │
        │                               │
        │──── LOCK_RELEASE ────────────►│
        │                               │ (libera, notifica siguiente en cola)
```

Si el lock está ocupado cuando llega una petición: el maestro encola la petición y responde `LOCK_DENY`. Cuando el lock se libera, el maestro envía `LOCK_GRANT` al siguiente en la cola automáticamente.

### Elección de líder (Algoritmo Bully)

Cuando el maestro no responde a los heartbeats, el nodo que lo detecta inicia una elección. El nodo con el mayor ID entre los activos gana.

```
Nodo 2 detecta que nodo 1 (maestro) está caído:

Nodo 2                   Nodo 3
  │                         │
  │──── ELECTION ──────────►│   (nodo 3 tiene ID mayor)
  │                         │
  │◄─── ELECTION_OK ────────│   (nodo 3 dice "yo me encargo")
  │                         │
  │                         │──── ELECTION ────► (no hay nodos con ID > 3)
  │                         │
  │                         │   (declara victoria — 3s sin respuesta)
  │                         │
  │◄─── COORDINATOR ────────│   (nodo 3 es el nuevo maestro)
  │                         │
  │  actualiza masterID=3   │
```

### Heartbeat (detección de fallas)

Cada nodo envía un `PING` a todos sus pares cada 5 segundos. Si un nodo no responde `PONG` en 3 intentos consecutivos (15 segundos), se declara muerto.

```
nodo1 ──PING──► nodo2 ──PONG──► nodo1    ✓ vivo, missed=0
nodo1 ──PING──► nodo2            (timeout) missed=1
nodo1 ──PING──► nodo2            (timeout) missed=2
nodo1 ──PING──► nodo2            (timeout) missed=3  →  NODO MUERTO

Si nodo2 era el maestro → iniciar elección
Si nodo2 era sucursal   → maestro redistribuye tickets abiertos de nodo2
```

---

## Flujo completo: levantar un ticket

```
Usuario en sucursal2:
  CLI → RaiseTicket(id_usuario=5, id_dispositivo=12)

1. EXCLUSIÓN MUTUA
   sucursal2 ──LOCK_REQUEST──► maestro(sucursal1)
   sucursal1 ──LOCK_GRANT────► sucursal2
   (lock adquirido)

2. BUSCAR INGENIERO DISPONIBLE
   sucursal2 ──QUERY(INGENIEROS)──► sucursal1, sucursal3
   sucursal1 ──QUERY_RESPONSE(ing1 disponible=1)──► sucursal2
   sucursal3 ──QUERY_RESPONSE(ing5 disponible=1)──► sucursal2
   sucursal2 elige ing1 (primer disponible)

3. CONSENSO — INSERTAR TICKET
   sucursal2 ──PROPOSE(INSERT_TICKET, {id=X, usuario=5, ingeniero=ing1, sucursal=2})──► todos
   todos ──VOTE_YES──► sucursal2
   sucursal2 ──COMMIT──► todos
   cada nodo ejecuta commit handler:
     - sucursal2: INSERT en TICKETS (sucursal_id=2 == self.ID) ✓
     - sucursal1: no inserta (sucursal_id=2 ≠ 1)
     - sucursal3: no inserta (sucursal_id=2 ≠ 3)

4. GENERAR FOLIO
   crea archivo: tickets/5-ing1-2-X.txt

5. CONSENSO — GUARDAR FOLIO
   sucursal2 ──PROPOSE(UPDATE_TICKET_FOLIO, {id=X, folio="5-ing1-2-X.txt"})──► todos
   todos ──VOTE_YES──► sucursal2
   sucursal2 ──COMMIT──► todos
   cada nodo actualiza si tiene el ticket

6. CONSENSO — MARCAR INGENIERO OCUPADO
   sucursal2 ──PROPOSE(UPDATE_INGENIERO_DISPONIBLE, {id=ing1, disponible=0})──► todos
   todos ──VOTE_YES──► sucursal2
   sucursal2 ──COMMIT──► todos
   sucursal1 actualiza ing1.disponible=0 en su DB (es suyo)

7. LIBERAR LOCK
   sucursal2 ──LOCK_RELEASE──► sucursal1
   sucursal1 libera lock (o notifica siguiente en cola)

Resultado: ticket creado, ingeniero asignado, folio generado, todos los nodos consistentes.
```

---

## Flujo completo: cerrar un ticket

```
Ingeniero en sucursal1:
  CLI → CloseTicket(id_ticket=X, id_ingeniero=ing1)

1. CONSENSO — CERRAR TICKET
   sucursal1 ──PROPOSE(CLOSE_TICKET, {id_ticket=X, id_ingeniero=ing1})──► todos
   todos ──VOTE_YES──► sucursal1
   sucursal1 ──COMMIT──► todos
   cada nodo:
     - UPDATE TICKETS SET estado='CERRADO' WHERE id=X (no-op si no lo tiene)
     - UPDATE INGENIEROS SET disponible=1 WHERE id=ing1 (aplica en quien lo tenga)

Resultado: ticket cerrado, ingeniero disponible nuevamente.
```

---

## Flujo: agregar dispositivo

```
Cualquier sucursal:
  CLI → AddDevice(nombre="Laptop-001", tipo="Laptop")

Si self es maestro:
  1. Consulta DISPOSITIVOS a todos → cuenta dispositivos por sucursal
  2. Elige la sucursal con menos dispositivos (sucursal_id = target)
  3. CONSENSO — INSERT_DISPOSITIVO con sucursal_id=target
  4. Solo target inserta en su SQLite

Si self NO es maestro:
  1. Envía ADD_DEVICE al maestro
  2. Maestro ejecuta el flujo anterior
```

---

## Flujo: falla de un nodo sucursal

```
sucursal3 cae (proceso muerto)

sucursal1 y sucursal2 detectan via heartbeat (15s):
  missed[3] = 3 → declarar muerto → state.MarkDead(3)

Si sucursal1 es maestro:
  heartbeat.OnNodeDead(3) → service.RedistributeTickets(ctx, 3)
    1. Consulta local: tickets con sucursal_id=3 y estado='ABIERTO'
    2. Consulta INGENIEROS a nodos vivos → busca disponible≠sucursal3
    3. Por cada ticket abierto: PROPOSE(REASSIGN_TICKET, nuevo_ingeniero)
    4. COMMIT en todos los nodos vivos

Si sucursal2 detecta antes que sucursal1:
  Notifica al maestro: PROPOSE(NODE_DEAD, {dead_node_id=3})
  Maestro maneja redistribución
```

---

## Flujo: falla del nodo maestro

```
sucursal1 (maestro) cae

sucursal2 detecta via heartbeat:
  missed[1] = 3 → declarar muerto
  id muerto == masterID → iniciar elección

Bully entre sucursal2 (id=2) y sucursal3 (id=3):
  sucursal2 envía ELECTION a sucursal3
  sucursal3 responde ELECTION_OK
  sucursal3 se declara ganadora → envía COORDINATOR a sucursal2
  sucursal2 actualiza masterID=3

Ahora sucursal3 es el nuevo maestro:
  - Gestiona locks de mutex
  - Redistribuye tickets de sucursal1
  - Responde heartbeats como maestro
```

---

## Base de datos (SQLite por nodo)

### Tablas

```sql
INGENIEROS (id, nombre, sucursal_id, disponible)
USUARIOS   (id, nombre, sucursal_id)
DISPOSITIVOS (id, nombre, tipo, sucursal_id)
TICKETS    (id, id_usuario, id_ingeniero, id_sucursal, id_dispositivo,
            estado, folio, created_at, closed_at)
_schema_version (version)   ← control de migraciones
```

### Migraciones

Al iniciar, `db.Open()` verifica la versión en `_schema_version` y aplica las migraciones pendientes en orden. Al arrancar se imprime:

```
[sucursal1] db schema version: 1
```

Para agregar cambios al esquema: agregar entrada al slice `migrations` en `internal/db/db.go` con el siguiente número de versión.

---

## Folio de ticket

Formato del nombre de archivo: `IDUSUARIO-IDINGENIERO-IDSUCURSAL-IDTICKET.txt`

Ejemplo: `5-101-2-987654321.txt`

Contenido:
```
FOLIO: 5-101-2-987654321.txt
Usuario: 5
Ingeniero: 101
Sucursal: 2
Ticket: 987654321
Fecha: 2026-05-06T18:30:00Z
```

---

## Tipos de mensajes TCP

Todos los mensajes tienen la estructura:
```json
{
  "id": "uuid",
  "type": "TIPO_MENSAJE",
  "from_node": "sucursal2",
  "to_node": "sucursal1",
  "content": "{...payload JSON...}",
  "send_at": "2026-05-06T18:30:00Z"
}
```

| Categoría | Tipos |
|-----------|-------|
| Mensajería legacy | `MSG`, `BROADCAST` |
| Heartbeat | `PING`, `PONG` |
| Consenso | `PROPOSE`, `VOTE_YES`, `VOTE_NO`, `COMMIT`, `COMMIT_ACK` |
| Exclusión mutua | `LOCK_REQUEST`, `LOCK_GRANT`, `LOCK_RELEASE`, `LOCK_DENY` |
| Elección | `ELECTION`, `ELECTION_OK`, `COORDINATOR` |
| Consultas | `QUERY`, `QUERY_RESPONSE` |
| Dispositivos | `ADD_DEVICE`, `DISTRIBUTE` |
| Tickets | `ASSIGN_TICKET`, `CLOSE_TICKET` |
| Fallas | `NODE_DEAD`, `REDISTRIBUTE` |

---

## Configuración de nodos (`nodes.json`)

**Dev local (1 máquina, 3 nodos en el mismo proceso):**
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

**VMs (1 proceso por máquina) — campo `host` diferente en cada VM:**
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
