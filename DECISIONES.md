# Decisiones de Diseño y Trade-offs

Sistemas Distribuidos — Grupo 5, 26-2 — Segundo Entregable

Este documento justifica las decisiones técnicas del sistema. Para cada decisión se describe qué se eligió, por qué, y qué se sacrifica.

---

## 1. Base de datos: SQLite

### Alternativas consideradas
| Opción | Por qué se descartó |
|--------|---------------------|
| PostgreSQL | Requiere servidor separado en cada VM; agrega operación de un proceso externo que puede fallar independientemente del nodo |
| MySQL/MariaDB | Mismo problema que PostgreSQL |
| Cassandra | Tiene consenso y replicación integrados, pero no hace elección de líder ni exclusión mutua — el middleware tendría que implementarlos de todas formas; además la curva de aprendizaje es alta |
| Archivo JSON/CSV | Sin transacciones, sin queries, sin integridad referencial |

### Decisión: SQLite
SQLite corre embebido en el mismo proceso Go. No hay proceso externo que mantener. Si el nodo Go muere, la DB queda consistente (SQLite usa WAL). Un solo archivo por nodo, fácil de respaldar y entregar en USB.

**Lo que se sacrifica:** SQLite no soporta escrituras concurrentes desde múltiples procesos. Esto no es problema aquí porque cada nodo es el único proceso que escribe su propia DB — todas las escrituras pasan por consenso y llegan serializadas al commit handler.

---

## 2. Distribución: fragmentación, no replicación

### Alternativas consideradas
- **Replicación:** cada nodo tiene copia de todos los datos. Lecturas rápidas desde cualquier nodo. Requiere protocolo de sincronización para cada escritura en todas las copias.
- **Fragmentación (sharding):** cada nodo guarda solo sus filas (`sucursal_id = self.id`). Más simple. Lecturas distribuidas requieren broadcast.

### Decisión: fragmentación
El profesor confirmó que replicación no es requisito. Fragmentar es suficiente y más simple de implementar correctamente. El consenso garantiza que cada escritura llegue al nodo correcto; las consultas distribuidas (`QUERY` / `QUERY_RESPONSE`) agregan los fragmentos cuando se necesita la vista completa.

**Lo que se sacrifica:** una consulta global (ej. "todos los ingenieros") requiere broadcast a todos los nodos y esperar respuesta. Con replicación sería local. El timeout de 3 segundos absorbe nodos lentos.

---

## 3. Consenso: quórum simple, no Raft ni Paxos

### Alternativas consideradas
| Protocolo | Descripción | Por qué no |
|-----------|-------------|------------|
| Paxos | Consenso distribuido clásico, muy robusto | Extremadamente complejo de implementar correctamente; fases de prepare/promise/accept/commit con muchos casos borde |
| Raft | Más legible que Paxos, usado en etcd/CockroachDB | Requiere log replicado, snapshots, gestión de términos — scope mayor al proyecto |
| Quórum simple (elegido) | Propose → Vote → Commit con mayoría | Implementable en tiempo de proyecto, cumple el requisito de consistencia |

### Decisión: quórum simple (Propose → Vote → Commit)
Cumple el requisito del profesor ("debe haber consenso") con complejidad manejable. La regla es clara: una escritura necesita N/2 + 1 votos para proceder. Si el proponente cae antes del COMMIT, ningún nodo aplica la operación — rollback natural sin log de recuperación.

**Lo que se sacrifica:** no hay recuperación de rondas incompletas si el proponente cae durante el COMMIT (algunos nodos podrían aplicar, otros no). Raft resolvería esto con log replicado. Para el scope del proyecto, este caso borde es aceptable.

---

## 4. Elección de líder: Bully algorithm

### Alternativas consideradas
| Algoritmo | Descripción | Por qué no |
|-----------|-------------|------------|
| Ring election | Mensajes circulan en anillo; gana el mayor ID | Requiere topología en anillo, más difícil de mantener con nodos que entran/salen |
| Raft leader election | Basada en términos y votos | Acoplada al log replicado de Raft; no aplica sin el resto de Raft |
| Bully (elegido) | El nodo con mayor ID activo gana | Simple, sin topología especial, fácil de razonar |

### Decisión: Bully algorithm
Cada nodo conoce los IDs de todos los demás (configuración estática). Al detectar que el maestro cayó, el nodo inicia elección enviando `ELECTION` a nodos con ID mayor. Si nadie responde en 3 segundos, se declara ganador. Fácil de depurar y verificar.

**Lo que se sacrifica:** Bully genera O(n²) mensajes en el peor caso (todos inician elección simultáneamente). Con 4 nodos esto es insignificante. En un sistema con cientos de nodos sería un problema.

---

## 5. Exclusión mutua: lock centralizado en el maestro

### Alternativas consideradas
| Mecanismo | Descripción | Por qué no |
|-----------|-------------|------------|
| Algoritmo de Ricart-Agrawala | Lock distribuido por timestamps lógicos | Requiere relojes lógicos (Lamport), O(n) mensajes por adquisición, más complejo |
| Token ring | Token circula; quien lo tiene puede escribir | Requiere anillo, el token puede perderse si un nodo cae |
| Lock centralizado (elegido) | El maestro es el árbitro único del lock | Simple, determinista, fácil de razonar |

### Decisión: lock centralizado en el maestro
Un solo nodo (el maestro) otorga y gestiona el lock. Cola FIFO para peticiones pendientes. Si el maestro cae, el algoritmo Bully elige uno nuevo y el lock se resetea. El acoplamiento entre mutex y elección de líder es explícito y controlado.

**Lo que se sacrifica:** el maestro es un punto único de falla para el lock. Si el maestro cae mientras alguien tiene el lock, ese lock se pierde y la operación en curso falla (se reintenta). Ricart-Agrawala no tiene este problema, pero es más difícil de implementar.

---

## 6. Comunicación entre nodos: TCP con JSON

### Alternativas consideradas
| Protocolo | Por qué no |
|-----------|------------|
| HTTP/REST | Overhead de headers, no mantiene conexión; latencia mayor por TCP handshake en cada request |
| gRPC | Excelente para producción, pero requiere definir schemas Protobuf y añade dependencia pesada |
| WebSockets | Diseñado para browser-server, no node-node |
| TCP + JSON (elegido) | Conexiones persistentes, formato legible, implementable desde cero |

### Decisión: TCP con JSON línea por línea
Cada nodo mantiene un pool de conexiones TCP persistentes a sus pares. Los mensajes son structs Go serializados como JSON, una línea por mensaje (`\n` como delimitador). Fácil de depurar con `telnet` o `nc`. Sin dependencias de terceros para el protocolo.

**Lo que se sacrifica:** JSON es más lento y pesado que Protobuf. Para 4 nodos con mensajes pequeños, esta diferencia es imperceptible. En un sistema con miles de mensajes por segundo, Protobuf o MessagePack serían mejores.

---

## 7. Identificadores distribuidos: prefijo por nodo + contador atómico

### Alternativas consideradas
| Esquema | Por qué no |
|---------|------------|
| UUID v4 | Sin colisiones, pero no permite saber en qué nodo vive el dato sin consulta extra |
| Auto-increment de SQLite | Colisiona entre nodos si dos nodos insertan simultáneamente |
| Snowflake ID | Requiere timestamp sincronizado (NTP obligatorio) y bit layout específico |
| Prefijo + contador atómico (elegido) | Sin colisiones, el nodo propietario es derivable del ID |

### Decisión: `nodeID × 10_000,000,000 + contador_atómico`
El ID codifica el nodo propietario. Dado `id = 20_000_000_005`, se sabe que pertenece a sucursal2 (`id / 10_000_000_000 = 2`). El contador atómico (`sync/atomic`) garantiza unicidad dentro del nodo sin mutex. Nodos distintos tienen prefijos distintos, nunca colisionan.

**Lo que se sacrifica:** el espacio de IDs por nodo está limitado a 10,000,000,000 registros. Para un sistema de tickets de soporte, esto es más que suficiente.

---

## 8. Tolerancia a fallas: consistencia sobre disponibilidad

El profesor preguntó explícitamente: *"¿Siempre priorizamos la disponibilidad o priorizamos la consistencia?"* La respuesta fue consistencia.

**Implicación práctica:** si no hay quórum (más de la mitad de los nodos caídos), el sistema rechaza escrituras en lugar de aceptarlas con riesgo de inconsistencia. Un ticket no se crea si no hay mayoría de nodos disponibles para votar.

Esto es coherente con el teorema CAP: al requerir consenso (C), se sacrifica disponibilidad total (A) bajo partición de red (P).

**Lo que se sacrifica:** si 3 de 4 nodos caen, el sistema queda en modo lectura de facto. Para alta disponibilidad se necesitaría replicación y un protocolo como Raft que permita operar con quórum más pequeño.

---

## Resumen de decisiones

| Decisión | Elegido | Principal trade-off |
|----------|---------|---------------------|
| Base de datos | SQLite embebido | Sin escrituras concurrentes entre procesos (no aplica aquí) |
| Distribución | Fragmentación | Lecturas globales requieren broadcast |
| Consenso | Quórum simple | Sin recuperación de rondas incompletas |
| Elección de líder | Bully | O(n²) mensajes (irrelevante con 4 nodos) |
| Exclusión mutua | Lock centralizado en maestro | Maestro es punto único de falla para el lock |
| Comunicación | TCP + JSON | Más lento que Protobuf (irrelevante a esta escala) |
| IDs | Prefijo de nodo + contador | Espacio limitado a 10B IDs por nodo |
| CAP | Consistencia (CP) | Sistema no acepta escrituras sin quórum |
