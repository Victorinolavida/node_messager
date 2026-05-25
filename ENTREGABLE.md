# Sistema Distribuido de Tickets de Soporte

**Segundo y último entregable — Sistemas Distribuidos, Grupo 5, 26-2**

---

## 1. Manual de Usuario

### 1.1 Requisitos

- Go 1.21 o superior
- No requiere CGO (SQLite puro en Go)

### 1.2 Instalación y ejecución

```bash
git clone <repo>
cd node_messager
go mod tidy
cp nodes-ejemplo.json nodes.json
go run ./cmd
```

Al iniciar, el sistema levanta 4 nodos (sucursales), cada uno con su servidor TCP y su base de datos SQLite independiente. En consola se confirma la conexión entre nodos:

> **[AQUÍ IMAGEN: Arranque del sistema mostrando los 4 nodos iniciando con sus versiones de esquema de BD y conexiones TCP establecidas]**

### 1.3 Menú principal

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
 12) add device            Agregar dispositivo (maestro distribuye)
 13) list all users        Ver todos los usuarios (todas las sucursales)
 14) list all engineers    Ver todos los ingenieros (todas las sucursales)
 15) list all devices      Ver todos los dispositivos (todas las sucursales)

  6) quit
```

### 1.4 Operaciones principales

#### Agregar usuario (opción 10) e ingeniero (opción 11)

Se ingresa el nombre y el sistema lo registra en la sucursal local mediante consenso (todos los nodos deben aprobar la escritura).

> **[AQUÍ IMAGEN: Creación de un usuario y un ingeniero desde el menú]**

#### Agregar dispositivo (opción 12)

Se ingresa nombre y tipo. El maestro consulta a todas las sucursales cuántos dispositivos tiene cada ingeniero y lo asigna al que tenga menos, logrando distribución equitativa.

> **[AQUÍ IMAGEN: Creación de un dispositivo mostrando la asignación automática al ingeniero con menos carga]**

#### Levantar ticket (opción 7)

Se ingresa el ID del usuario y del dispositivo. El sistema automáticamente:
1. Adquiere el lock de exclusión mutua.
2. Busca un ingeniero disponible en todas las sucursales.
3. Crea el ticket por consenso.
4. Genera el folio (`IDUSUARIO-IDINGENIERO-IDSUCURSAL-IDTICKET`).
5. Marca al ingeniero como ocupado.
6. Libera el lock.

> **[AQUÍ IMAGEN: Proceso de levantar un ticket mostrando la salida con el folio generado]**

#### Cerrar ticket (opción 8)

Se ingresa el ID del ticket y del ingeniero. El sistema cierra el ticket y libera al ingeniero, todo por consenso.

> **[AQUÍ IMAGEN: Cierre de un ticket desde la opción 8]**

#### Consultas distribuidas (opciones 9, 13, 14, 15)

Estas opciones envían un `QUERY` a todos los nodos, cada uno responde con sus registros locales, y el nodo que preguntó combina los resultados.

> **[AQUÍ IMAGEN: Listado de tickets (opción 9) mostrando folio, estado, usuario e ingeniero asignado]**

### 1.5 Identificadores

Para evitar colisiones entre nodos, los IDs usan un prefijo por sucursal:

```
sucursal1: 10000000001, 10000000002, ...
sucursal2: 20000000001, 20000000002, ...
sucursal3: 30000000001, 30000000002, ...
sucursal4: 40000000001, 40000000002, ...
```

Esto permite saber a qué nodo pertenece un registro solo viendo su ID.

---

## 2. Descripción del Proceso

**Un usuario levanta un ticket de soporte a un dispositivo, el sistema asigna a un ingeniero, este lo atiende y cierra el ticket.**

### Diagrama del proceso

```
  USUARIO                    SISTEMA (nodos distribuidos)              INGENIERO
     │                                  │                                  │
     │  1. Levanta ticket               │                                  │
     │     (id_usuario, id_dispositivo) │                                  │
     │─────────────────────────────────►│                                  │
     │                                  │                                  │
     │                    2. Adquiere lock (exclusión mutua)               │
     │                       Solo un nodo puede asignar a la vez          │
     │                                  │                                  │
     │                    3. Consulta distribuida: busca ingeniero         │
     │                       disponible en TODAS las sucursales            │
     │                                  │                                  │
     │                    4. Consenso: crea el ticket                      │
     │                       (mayoría de nodos debe aprobar)              │
     │                                  │                                  │
     │                    5. Genera folio y lo guarda (consenso)           │
     │                       IDUSUARIO-IDINGENIERO-IDSUCURSAL-IDTICKET    │
     │                                  │                                  │
     │                    6. Marca ingeniero ocupado (consenso)            │
     │                                  │                                  │
     │                    7. Libera lock                                   │
     │                                  │                                  │
     │  ✓ Ticket creado con folio       │                                  │
     │◄─────────────────────────────────│                                  │
     │                                  │                                  │
     │                                  │    8. El ingeniero atiende       │
     │                                  │       el dispositivo             │
     │                                  │                                  │
     │                                  │    9. Cierra el ticket           │
     │                                  │◄─────────────────────────────────│
     │                                  │       (consenso)                 │
     │                                  │                                  │
     │                                  │   10. Ingeniero queda disponible │
     │                                  │                                  │
     │  ✓ Ticket cerrado               │    ✓ Ticket cerrado             │
     │◄─────────────────────────────────│─────────────────────────────────►│
```

### ¿Por qué se necesitan mecanismos distribuidos en este proceso?

| Paso | Problema sin mecanismo | Mecanismo usado | Qué resuelve |
|------|------------------------|-----------------|--------------|
| 2 | Dos nodos asignan al mismo ingeniero simultáneamente | **Exclusión mutua** (lock centralizado en el maestro) | Solo un nodo a la vez puede buscar y asignar ingeniero |
| 3 | Los ingenieros están repartidos en distintas sucursales | **Consulta distribuida** (QUERY/QUERY_RESPONSE) | Combina datos de todos los nodos para tener la vista completa |
| 4-6 | Un nodo escribe sin que los demás se enteren | **Consenso por quórum** (PROPOSE → VOTE → COMMIT) | La mayoría aprueba antes de persistir, todos quedan consistentes |
| — | El maestro cae y nadie gestiona locks | **Elección Bully inverso** | El nodo con menor ID activo asume como nuevo maestro |
| — | Una sucursal cae con tickets abiertos | **Heartbeat + redistribución** | Se detecta la falla y se reasignan tickets a ingenieros vivos |

---

## 3. Evidencias

### 3.1 Datos iniciales: usuarios, ingenieros y dispositivos

> **[AQUÍ IMAGEN: Creación de usuarios en distintas sucursales (opción 10)]**

> **[AQUÍ IMAGEN: Creación de ingenieros en distintas sucursales (opción 11)]**

> **[AQUÍ IMAGEN: Adición de dispositivos (opción 12) mostrando que el maestro los distribuye equitativamente entre ingenieros]**

### 3.2 Consultas distribuidas

> **[AQUÍ IMAGEN: Listado de todos los usuarios (opción 13) — se ven datos de todas las sucursales combinados]**

> **[AQUÍ IMAGEN: Listado de todos los ingenieros (opción 14) — se ve el campo "disponible" de cada uno]**

> **[AQUÍ IMAGEN: Listado de todos los dispositivos (opción 15) — se ve la distribución entre sucursales]**

### 3.3 Levantar ticket (proceso completo)

> **[AQUÍ IMAGEN: Levantamiento de ticket (opción 7) mostrando el usuario, dispositivo y el folio generado al final]**

### 3.4 Folio generado

> **[AQUÍ IMAGEN: Listado de tickets (opción 9) donde se vea el folio con formato IDUSUARIO-IDINGENIERO-IDSUCURSAL-IDTICKET y estado ABIERTO]**

### 3.5 Cierre de ticket

> **[AQUÍ IMAGEN: Cierre de ticket (opción 8) y posterior listado mostrando el estado cambiado a CERRADO]**

### 3.6 Exclusión mutua en acción

> **[AQUÍ IMAGEN: Logs mostrando dos nodos intentando levantar ticket al mismo tiempo — uno obtiene LOCK_GRANT y el otro LOCK_DENY hasta que el primero libera]**

### 3.7 Consenso en acción

> **[AQUÍ IMAGEN: Logs TCP mostrando los mensajes PROPOSE, VOTE_YES y COMMIT durante una escritura]**

### 3.8 Caída de una sucursal y redistribución

> **[AQUÍ IMAGEN: Detección de nodo caído vía heartbeat (3 PINGs sin PONG) y redistribución automática de sus tickets abiertos a ingenieros disponibles en nodos activos]**

### 3.9 Elección de líder

> **[AQUÍ IMAGEN: Caída del nodo maestro, proceso de elección Bully inverso (ELECTION → ELECTION_OK → COORDINATOR) y nuevo maestro asumiendo el rol]**

---

## 4. Conclusiones en Base a los Requerimientos

### Req. 1 — 3 o 4 nodos (sucursales)

**Cumplido.** El sistema opera con 4 sucursales. Cada nodo tiene su propio servidor TCP en un puerto distinto y su base de datos SQLite independiente. Se eligió esta cantidad porque es suficiente para demostrar todos los mecanismos distribuidos (quórum con mayoría, elección con múltiples candidatos, redistribución entre nodos).

### Req. 2 — Base de datos distribuida con tablas INGENIEROS, USUARIOS, DISPOSITIVOS, TICKETS

**Cumplido.** Cada nodo tiene las 4 tablas pero solo almacena los registros que le pertenecen (fragmentación por `sucursal_id`). Se eligió fragmentación en lugar de replicación porque es más simple de implementar correctamente y el consenso ya garantiza que cada escritura llegue al nodo correcto. Las consultas globales se resuelven con broadcast (`QUERY`/`QUERY_RESPONSE`).

### Req. 3 — Que el nodo maestro distribuya automáticamente los dispositivos entre las sucursales

**Cumplido.** El maestro consulta cuántos dispositivos tiene cada ingeniero en todas las sucursales y asigna el nuevo dispositivo al ingeniero con menos carga. Si la petición viene de otro nodo, se reenvía al maestro automáticamente vía mensaje `ADD_DEVICE`.

### Req. 4 — Consultar y actualizar la lista de usuarios e ingenieros (DISTRIBUIDA) en cualquier sucursal

**Cumplido.** Las opciones 13 y 14 envían `QUERY` a todos los nodos y combinan las respuestas. Cualquier sucursal puede agregar usuarios (opción 10) e ingenieros (opción 11) mediante consenso, y los cambios se reflejan inmediatamente en consultas desde cualquier nodo.

### Req. 5 — Levantar un ticket desde cualquier sucursal (exclusión mutua)

**Cumplido.** Cualquier sucursal puede levantar un ticket (opción 7). Se eligió un lock centralizado en el maestro porque es simple y determinista: el maestro mantiene una cola FIFO de peticiones. Si el maestro cae, la elección Bully elige uno nuevo y el lock se reinicia. La alternativa (Ricart-Agrawala) era más compleja sin beneficio real para 4 nodos.

### Req. 5.1 — Exclusión mutua para no asignar un ingeniero 2 veces

**Cumplido.** El lock garantiza que solo un nodo puede buscar y asignar ingeniero a la vez. Si nodo1 tiene el lock y asigna al ingeniero A, cuando nodo2 obtiene el lock ya ve al ingeniero A como `disponible=0` y elige otro. Sin este mecanismo, ambos nodos podrían ver al mismo ingeniero como disponible y asignarlo dos veces.

### Req. 6 — Cerrar un ticket desde cualquier sucursal

**Cumplido.** Cualquier sucursal puede cerrar un ticket (opción 8) mediante consenso. La operación `CLOSE_TICKET` actualiza el estado a `CERRADO` y marca al ingeniero como `disponible=1`. Cada nodo aplica la operación solo si tiene el registro correspondiente en su fragmento.

### Req. 7 — Generar y guardar el folio (IDUSUARIO+IDINGENIERO+SUCURSAL+IDTICKET)

**Cumplido.** El folio se genera como concatenación `IDUSUARIO-IDINGENIERO-IDSUCURSAL-IDTICKET` y se almacena en la columna `folio` de la tabla TICKETS mediante consenso (`UPDATE_TICKET_FOLIO`). Se eligió guardarlo en la base de datos (no en archivo externo) porque así participa del mismo mecanismo de consenso y no hay riesgo de desincronización.

### Req. 8 — Agregar dispositivos desde cualquier sucursal con distribución equitativa

**Cumplido.** Desde cualquier sucursal se solicita agregar un dispositivo. El maestro cuenta los dispositivos por ingeniero en todas las sucursales y asigna al que tenga menos. Esto se logra con consulta distribuida + consenso para la inserción.

### Req. 9 — Cada actualización a los datos debe tener consenso

**Cumplido.** Toda escritura pasa por `PROPOSE → VOTE → COMMIT` con quórum de `(nodos_activos / 2) + 1`. Se eligió un quórum simple en lugar de Raft o Paxos porque cumple el requisito de consistencia con complejidad manejable. El sistema prioriza consistencia sobre disponibilidad (CP en el teorema CAP): si no hay quórum, la escritura se rechaza.

### Req. 10 — Si una sucursal falla, redistribuir los soportes y actualizar la información

**Cumplido.** Cada nodo envía `PING` a sus pares cada 5 segundos. Si no recibe `PONG` en 3 intentos consecutivos (15 segundos), declara al nodo muerto. El maestro redistribuye automáticamente los tickets abiertos del nodo caído, reasignándolos a ingenieros disponibles en nodos activos mediante consenso.

### Req. 11 — Si el nodo maestro falla, debe haber elección

**Cumplido.** Se implementó el algoritmo Bully inverso: el nodo con menor ID activo gana (mayor prioridad). Cuando se detecta que el maestro no responde, se envían mensajes `ELECTION` a nodos con ID menor. Si nadie responde en 3 segundos, el nodo se declara maestro y envía `COORDINATOR` a todos. Se eligió Bully inverso sobre Ring Election porque no requiere topología especial.

### Req. 12 — Descripción del proceso completo

**Cumplido.** El flujo completo se documenta en la sección 2. El proceso integra los cuatro mecanismos distribuidos: exclusión mutua (asignación sin duplicados), consulta distribuida (buscar ingeniero), consenso (persistir cada cambio) y tolerancia a fallas (heartbeat + elección).

---

### Tabla resumen

| #    | Requerimiento                                            | Estado   |
|------|----------------------------------------------------------|----------|
| 1    | 3 o 4 nodos (sucursales)                                | Cumplido |
| 2    | BD distribuida: INGENIEROS, USUARIOS, DISPOSITIVOS, TICKETS | Cumplido |
| 3    | Maestro distribuye dispositivos automáticamente          | Cumplido |
| 4    | Consultar/actualizar usuarios e ingenieros (distribuida) | Cumplido |
| 5    | Levantar ticket desde cualquier sucursal                 | Cumplido |
| 5.1  | Exclusión mutua al asignar ingeniero                     | Cumplido |
| 6    | Cerrar ticket desde cualquier sucursal                   | Cumplido |
| 7    | Generar y guardar folio del ticket                       | Cumplido |
| 8    | Agregar dispositivos con distribución equitativa         | Cumplido |
| 9    | Consenso en cada actualización de datos                  | Cumplido |
| 10   | Redistribución ante falla de sucursal                    | Cumplido |
| 11   | Elección si el maestro falla                             | Cumplido |
| 12   | Proceso completo: levantar → asignar → cerrar           | Cumplido |
