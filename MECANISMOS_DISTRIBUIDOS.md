# Mecanismos Distribuidos — Guía Técnica

Este documento explica cómo el sistema resuelve los cuatro problemas clásicos de sistemas distribuidos y cómo fluyen los tickets de soporte de principio a fin.

---

## Índice

1. [Problema 1 — Consistencia: Consenso por Quórum](#1-consenso-por-quórum)
2. [Problema 2 — Exclusión Mutua: Lock Centralizado](#2-exclusión-mutua)
3. [Problema 3 — Tolerancia a Fallas: Heartbeat y Redistribución](#3-heartbeat-y-redistribución)
4. [Problema 4 — Elección de Líder: Algoritmo Bully (inverso)](#4-algoritmo-bully-inverso)
5. [Cómo se crea un ticket (flujo completo)](#5-flujo-completo-creación-de-ticket)
6. [Identificadores distribuidos](#6-identificadores-distribuidos)
7. [Consultas distribuidas](#7-consultas-distribuidas)

---

## 1. Consenso por Quórum

### Problema
Si el nodo 1 escribe un dato en su SQLite local sin coordinarse, el nodo 2 nunca lo sabrá. Los datos quedan inconsistentes.

### Solución: Propose → Vote → Commit

Antes de cualquier escritura, el nodo iniciador debe obtener aprobación de la **mayoría** de los nodos activos.

```
Quórum requerido = (nodos_activos / 2) + 1

Con 3 nodos: necesita 2 votos
Con 4 nodos: necesita 3 votos
```

### Flujo detallado

```
Nodo 2 quiere insertar un ticket:

Paso 1 — PROPOSE (broadcast a todos)
  nodo2 ──PROPOSE──► nodo1  { round_id: "abc", operation: "INSERT_TICKET", data: "{...}" }
  nodo2 ──PROPOSE──► nodo3  { round_id: "abc", operation: "INSERT_TICKET", data: "{...}" }
  nodo2 vota YES a sí mismo (cuenta como 1 voto)

Paso 2 — VOTE_YES (cada nodo valida y responde)
  nodo1 ──VOTE_YES──► nodo2  { round_id: "abc" }
  nodo3 ──VOTE_YES──► nodo2  { round_id: "abc" }

Paso 3 — quórum alcanzado (2 de 3 votos)
  nodo2 tiene 3 votos (propio + nodo1 + nodo3) ≥ 2 necesarios

Paso 4 — COMMIT (broadcast a todos)
  nodo2 ──COMMIT──► nodo1  { round_id: "abc", operation: "INSERT_TICKET", data: "{...}" }
  nodo2 ──COMMIT──► nodo3  { round_id: "abc", operation: "INSERT_TICKET", data: "{...}" }
  nodo2 aplica escritura local

Paso 5 — cada nodo aplica el commit
  nodo1: commitHandler("INSERT_TICKET", data) → id_sucursal=2 ≠ 1 → NO inserta
  nodo3: commitHandler("INSERT_TICKET", data) → id_sucursal=2 ≠ 3 → NO inserta
  nodo2 ya lo aplicó en el paso 4
```

### Timeout y fallos

```
Timeout de votos: 3 segundos

Si no llega quórum en 3s → error "no quorum reached"
Si un nodo cae durante la votación → los demás votan, si queda mayoría → commit
Si el proponente cae antes del COMMIT → ningún nodo aplica (rollback natural)
```

### Operaciones que usan consenso

| Operación | Mensaje PROPOSE |
|-----------|----------------|
| Agregar usuario | `INSERT_USUARIO` |
| Agregar ingeniero | `INSERT_INGENIERO` |
| Agregar dispositivo | `INSERT_DISPOSITIVO` |
| Crear ticket | `INSERT_TICKET` |
| Guardar folio | `UPDATE_TICKET_FOLIO` |
| Marcar ingeniero ocupado | `UPDATE_INGENIERO_DISPONIBLE` |
| Cerrar ticket | `CLOSE_TICKET` |
| Reasignar ticket | `REASSIGN_TICKET` |

### Código relevante

```
internal/consensus/consensus.go
  Engine.Propose()       — inicia la ronda, espera votos
  Engine.HandlePropose() — recibe PROPOSE, vota YES
  Engine.HandleVote()    — acumula votos, detecta quórum
  Engine.HandleCommit()  — aplica operación vía commitHandler
  Engine.doCommit()      — envía COMMIT a todos y aplica local
```

---

## 2. Exclusión Mutua

### Problema
Si el nodo 1 y el nodo 2 buscan un ingeniero disponible al mismo tiempo, ambos pueden elegir al mismo ingeniero y asignarle dos tickets simultáneamente.

```
SIN exclusión mutua:
  nodo1: busca → encuentra ing_A disponible → asigna ticket_1 a ing_A
  nodo2: busca → encuentra ing_A disponible → asigna ticket_2 a ing_A  ← ¡doble asignación!
```

### Solución: Lock centralizado en el maestro

El nodo maestro actúa como árbitro. Solo puede haber **un holder del lock** a la vez.

```
CON exclusión mutua:
  nodo2 ──LOCK_REQUEST──► maestro(nodo1)
  maestro: lock libre → otorga
  maestro ──LOCK_GRANT────► nodo2

  nodo2 tiene el lock → busca ingeniero → asigna ticket → libera

  nodo2 ──LOCK_RELEASE───► maestro
  maestro: libera lock → notifica siguiente en cola
```

### Flujo cuando el lock está ocupado

```
nodo2 tiene el lock. nodo3 también quiere asignar:

  nodo3 ──LOCK_REQUEST──► maestro
  maestro: lock ocupado → encola nodo3 → responde LOCK_DENY
  nodo3 espera (hasta 5 segundos)

  nodo2 termina → LOCK_RELEASE → maestro
  maestro: cola tiene nodo3 → LOCK_GRANT a nodo3
  nodo3 desbloquea → ahora puede asignar
```

### Cola de espera

```
maestro.holder = "req-nodo2"
maestro.queue  = [req-nodo3, req-nodo4, ...]

Liberación en orden FIFO: nodo3 → nodo4 → ...
```

### Timeout y fallos

```
Timeout de espera: 5 segundos
Si el maestro cae mientras alguien espera → Bully elige nuevo maestro
El nuevo maestro empieza con lock libre (ningún lock pendiente se recupera)
```

### Código relevante

```
internal/mutex/mutex.go
  Engine.Acquire()           — decide local o remoto según si es maestro
  Engine.acquireLocal()      — lock en memoria si self es maestro
  Engine.acquireRemote()     — envía LOCK_REQUEST y espera LOCK_GRANT
  Engine.releaseLocal()      — libera y notifica siguiente en cola
  Engine.HandleLockRequest() — maestro: otorga o encola
  Engine.HandleLockGrant()   — no-maestro: desbloquea goroutine esperando
  Engine.HandleLockRelease() — maestro: libera y sirve siguiente
```

---

## 3. Heartbeat y Redistribución

### Problema
Si un nodo cae, sus tickets abiertos quedan sin atender y no hay ingeniero disponible para resolverlos.

### Solución: PING/PONG + redistribución automática

Cada nodo envía PING a sus pares cada 5 segundos. Si no recibe PONG en 3 intentos (15 segundos totales), declara al nodo muerto.

```
Ciclo normal:
  nodo1 ──PING──► nodo2
  nodo2 ──PONG──► nodo1  ✓ vivo, missed=0

Nodo caído:
  nodo1 ──PING──► nodo2  (timeout 2s) → missed=1
  nodo1 ──PING──► nodo2  (timeout 2s) → missed=2
  nodo1 ──PING──► nodo2  (timeout 2s) → missed=3 → NODO 2 MUERTO
```

### Redistribución de tickets

Cuando el maestro detecta (o recibe notificación de) un nodo muerto:

```
nodo2 cae. nodo1 es maestro.

nodo1 detecta nodo2 muerto → service.RedistributeTickets(ctx, 2)

1. Consulta local: TICKETS donde sucursal_id=2 AND estado='ABIERTO'
   → encuentra ticket_5 (usuario=10, dispositivo=20)

2. ListAll("INGENIEROS") → todos los nodos responden con sus ingenieros
   → encuentra ing_C (sucursal=3, disponible=1)

3. PROPOSE("REASSIGN_TICKET", {
     id: ticket_5,
     id_sucursal: 1,     ← se transfiere al maestro
     id_ingeniero: ing_C
   })

4. Todos votan, COMMIT → ticket_5 ahora asignado a ing_C en nodo1
```

### Diagrama de decisión al detectar nodo muerto

```
nodo detecta peer muerto
        │
        ▼
¿es el maestro?
   │          │
  SÍ          NO
   │          │
   ▼          ▼
redistribuir  enviar NODE_DEAD al maestro
tickets       → maestro redistribuye
del nodo muerto
```

### Código relevante

```
internal/heartbeat/heartbeat.go
  Monitor.Run()         — goroutine: ticker cada 5s → pingAll()
  Monitor.pingOne()     — envía PING, espera PONG con timeout
  Monitor.declareDead() — marca muerto, decide acción
  Monitor.HandlePing()  — responde PONG inmediatamente
  Monitor.HandlePong()  — señaliza canal de espera en pingOne

internal/service/ticket_service.go
  TicketService.RedistributeTickets() — busca tickets huérfanos y reasigna
```

---

## 4. Algoritmo Bully (inverso)

### Problema
Si el nodo maestro cae, nadie gestiona el lock de exclusión mutua ni coordina la redistribución de tickets.

### Solución: Bully algorithm (inverso)

El nodo con el **menor ID** entre los activos se convierte en el nuevo maestro. Es una variante del algoritmo Bully clásico donde la prioridad es inversa: menor ID = mayor prioridad.

### Regla simple
```
"El que tiene menor ID gana (mayor prioridad)"
Si soy el menor activo → soy el maestro
Si hay alguien menor que yo → que él se encargue
```

### Flujo completo

```
nodo1 (maestro, ID=1) cae.
nodo3 (ID=3) detecta que nodo1 no responde:

Paso 1 — nodo3 inicia elección
  nodo3 envía ELECTION a todos con ID < 3 (mayor prioridad):
  nodo3 ──ELECTION──► nodo2  { candidate_id: 3 }

Paso 2 — nodo2 (ID=2 < 3) tiene mayor prioridad → responde OK
  nodo2 ──ELECTION_OK──► nodo3  (dice: "yo me encargo")
  nodo3 cancela su timer → se retira

Paso 3 — nodo2 inicia su propia elección
  nodo2 envía ELECTION a todos con ID < 2:
  (nodo1 está caído, nadie responde) → espera 3s sin respuesta

Paso 4 — nodo2 se declara ganador (menor ID activo)
  nodo2 ──COORDINATOR──► nodo3  { master_id: 2 }
  nodo3 actualiza masterID = 2

Resultado: nodo2 es el nuevo maestro.
  - Gestiona el lock de mutex
  - Redistribuye tickets de nodo1
  - Responde como maestro a futuros LOCK_REQUEST
```

### Caso: único candidato

```
Solo queda un nodo activo (nodo4):
  nodo4 envía ELECTION a nodos con ID < 4 → ninguno responde
  nodo4 no recibe ELECTION_OK en 3s
  nodo4 ──COORDINATOR──► (broadcast a todos vivos)
  nodo4 es el maestro
```

### Timeout de elección

```
Timeout de respuesta ELECTION_OK: 3 segundos

Si en 3s nadie con ID menor responde:
  → yo soy el menor activo → me declaro maestro
  → envío COORDINATOR a todos
```

### Código relevante

```
internal/election/election.go
  Engine.StartElection()    — inicia Bully inverso, envía ELECTION a nodos con ID menor
  Engine.declareVictory()   — me declaro maestro, envío COORDINATOR
  Engine.HandleElection()   — recibo ELECTION: si tengo menor ID, respondo OK y compito
  Engine.HandleElectionOK() — alguien con menor ID está activo → cancelo timer → me retiro
  Engine.HandleCoordinator()— actualizo masterID con el ganador
  Engine.lowerNodes()       — filtra nodos con ID < self.ID
```

---

## 5. Flujo completo: Creación de Ticket

Esta sección muestra paso a paso qué ocurre cuando un usuario levanta un ticket desde cualquier sucursal.

### Diagrama de secuencia

```
                  CLI (sucursal2)    Maestro (sucursal1)    sucursal3
                       │                    │                    │
[1] Usuario elige      │                    │                    │
    "raise ticket"     │                    │                    │
    id_usuario=10      │                    │                    │
    id_dispositivo=20  │                    │                    │
                       │                    │                    │
[2] Exclusión mutua    │                    │                    │
    ──LOCK_REQUEST────►│                    │                    │
                       │──LOCK_REQUEST─────►│                    │
                       │◄──LOCK_GRANT───────│                    │
                       │ (lock adquirido)   │                    │
                       │                    │                    │
[3] Buscar ingeniero   │                    │                    │
    ──QUERY────────────────────────────────►│                    │
    ──QUERY─────────────────────────────────────────────────────►│
                       │                    │                    │
                       │◄──QUERY_RESPONSE───│ {ing_A, disp=1}    │
                       │◄──QUERY_RESPONSE───────────────────────│ {ing_B, disp=0}
                       │                    │                    │
    elige ing_A        │                    │                    │
    (primer disponible)│                    │                    │
                       │                    │                    │
[4] Consenso — INSERT_TICKET               │                    │
    ticket = {         │                    │                    │
      id: 10_000_000_001                   │                    │
      id_usuario: 10   │                    │                    │
      id_ingeniero: id_ing_A               │                    │
      id_sucursal: 2   │                    │                    │
      id_dispositivo:20│                    │                    │
    }                  │                    │                    │
    ──PROPOSE─────────────────────────────►│                    │
    ──PROPOSE───────────────────────────────────────────────────►│
                       │◄──VOTE_YES─────────│                    │
                       │◄──VOTE_YES─────────────────────────────│
    (quórum 3/3)       │                    │                    │
    ──COMMIT──────────────────────────────►│ NO inserta (s≠1)   │
    ──COMMIT────────────────────────────────────────────────────►│ NO inserta (s≠3)
    aplica local       │                    │                    │
    (s=2 == self) ✓    │                    │                    │
                       │                    │                    │
[5] Generar folio      │                    │                    │
    folio = "10-{id_ing_A}-2-10_000_000_001"                    │
    (concatenación de IDs, almacenado en BD)                    │
                       │                    │                    │
[6] Consenso — UPDATE_TICKET_FOLIO         │                    │
    ──PROPOSE──────────────────────────────►│                    │
    ──PROPOSE───────────────────────────────────────────────────►│
    (votos → COMMIT)   │                    │                    │
    actualiza folio en │                    │                    │
    TICKETS local      │                    │                    │
                       │                    │                    │
[7] Consenso — UPDATE_INGENIERO_DISPONIBLE │                    │
    {id: id_ing_A, disponible: 0}          │                    │
    ──PROPOSE──────────────────────────────►│                    │
    ──PROPOSE───────────────────────────────────────────────────►│
    (votos → COMMIT)   │                    │                    │
    sucursal1 marca ing_A disponible=0 ✓   │                    │
                       │                    │                    │
[8] Liberar lock       │                    │                    │
    ──LOCK_RELEASE─────────────────────────►│                    │
                       │ (lock libre para  │                    │
                       │  siguiente)       │                    │
                       │                    │                    │
[9] CLI muestra:       │                    │                    │
    ✓ ticket raised    │                    │                    │
```

### Resultado final

```
sucursal2.db:
  TICKETS: id=10_000_000_001, usuario=10, ingeniero=ing_A,
           sucursal=2, dispositivo=20, estado=ABIERTO,
           folio="10-{id_ing_A}-2-10000000001"

sucursal1.db:
  INGENIEROS: ing_A.disponible = 0  (marca que está asignado)
```

### Cierre de ticket

```
Ingeniero en cualquier sucursal:
  CLI → CloseTicket(id_ticket=10_000_000_001, id_ingeniero=id_ing_A)

[1] Consenso — CLOSE_TICKET
    PROPOSE → VOTE → COMMIT a todos

[2] Cada nodo en handleCommit:
    UPDATE TICKETS SET estado='CERRADO', closed_at=now
    WHERE id=10_000_000_001
    (si el ticket no está en ese nodo → UPDATE afecta 0 filas → sin error)

    UPDATE INGENIEROS SET disponible=1
    WHERE id=id_ing_A
    (solo el nodo que tiene ing_A lo actualiza)

[3] Resultado: ticket cerrado, ing_A disponible nuevamente
```

---

## 6. Identificadores Distribuidos

### El problema de IDs duplicados

Si nodo1 y nodo2 generan IDs basados en tiempo simultáneamente → colisión.

### Solución: nodeID como prefijo + contador atómico

```
ID = nodeID × 10_000_000_000 + contador_atómico

nodo 1: 10_000_000_001, 10_000_000_002, 10_000_000_003 ...
nodo 2: 20_000_000_001, 20_000_000_002, 20_000_000_003 ...
nodo 3: 30_000_000_001, 30_000_000_002, 30_000_000_003 ...
```

**Garantías:**
- Mismo nodo → contador atómico (`sync/atomic`) → nunca se repite
- Nodos distintos → prefijo diferente → nunca colisionan
- Del ID se puede extraer el nodo propietario: `ownerNode = id / 10_000_000_000`

### Lookup por ID

```go
// saber en qué nodo está un ingeniero con id=20_000_000_005:
ownerNodeID := id / 10_000_000_000  // = 2
targetNode  := state.NodeByID(ownerNodeID)
// → sucursal2
```

Actualmente el sistema usa `ListAll` (broadcast a todos). La optimización futura sería rutear directamente al nodo propietario usando la extracción del ID.

---

## 7. Consultas Distribuidas

### Problema
Los datos están fragmentados. Para ver todos los ingenieros hay que preguntar a todos los nodos.

### Flujo de QUERY / QUERY_RESPONSE

```
CLI en sucursal2: "list all engineers"

sucursal2 ──QUERY──► sucursal1  { table: "queryID|INGENIEROS", requester_id: 2 }
sucursal2 ──QUERY──► sucursal3  { table: "queryID|INGENIEROS", requester_id: 2 }
sucursal2 también consulta su propia DB local

sucursal1 ──QUERY_RESPONSE──► sucursal2  { rows: [ing_A, ing_B], node_id: 1 }
sucursal3 ──QUERY_RESPONSE──► sucursal2  { rows: [ing_E],        node_id: 3 }

sucursal2 espera 3 segundos (timeout)
sucursal2 combina: local + respuesta_1 + respuesta_3

Resultado final: [ing_A, ing_B, ing_C, ing_D, ing_E]
```

### Timeout de consulta

```
Timeout: 3 segundos

Si un nodo tarda o no responde:
  → se usa lo que llegó hasta ese momento
  → nodos muertos no bloquean la consulta
```

### Tablas consultables por QUERY

| Tabla | Uso |
|-------|-----|
| `INGENIEROS` | Buscar disponibles para asignar ticket |
| `USUARIOS` | Listar todos los usuarios |
| `DISPOSITIVOS` | Contar dispositivos por nodo (distribución equitativa) |
| `TICKETS` | Ver todos los tickets del sistema |

---

## Resumen de mensajes TCP

| Mensaje | Dirección | Propósito |
|---------|-----------|-----------|
| `PING` | cualquier → peer | ¿estás vivo? |
| `PONG` | peer → cualquier | sí, estoy vivo |
| `PROPOSE` | iniciador → todos | propuesta de escritura |
| `VOTE_YES` | cualquier → iniciador | acepto la propuesta |
| `VOTE_NO` | cualquier → iniciador | rechazo la propuesta |
| `COMMIT` | iniciador → todos | aplicar la escritura |
| `LOCK_REQUEST` | cualquier → maestro | quiero el lock |
| `LOCK_GRANT` | maestro → solicitante | lock otorgado |
| `LOCK_DENY` | maestro → solicitante | lock ocupado, espera |
| `LOCK_RELEASE` | cualquier → maestro | libero el lock |
| `ELECTION` | candidato → nodos mayores | inicio elección |
| `ELECTION_OK` | nodo mayor → candidato | yo me encargo |
| `COORDINATOR` | ganador → todos | soy el nuevo maestro |
| `QUERY` | cualquier → todos | dame tus filas de tabla X |
| `QUERY_RESPONSE` | cualquier → solicitante | aquí están mis filas |
| `ADD_DEVICE` | cualquier → maestro | agrega este dispositivo |
| `NODE_DEAD` | detector → maestro | el nodo X está muerto |

---

## Glosario

| Término | Significado |
|---------|------------|
| **Quórum** | Mayoría de nodos vivos necesaria para aprobar una escritura |
| **Commit handler** | Función que aplica la operación en SQLite local cuando llega COMMIT |
| **Fragmentación** | Cada nodo guarda solo sus filas (`sucursal_id = self.ID`) |
| **Bully (inverso)** | Algoritmo de elección: gana el nodo con menor ID (mayor prioridad) |
| **Holder** | Nodo que actualmente tiene el lock de exclusión mutua |
| **Folio** | Identificador concatenado del ticket: `USUARIO-INGENIERO-SUCURSAL-TICKET`, almacenado en la BD |
