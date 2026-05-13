#!/usr/bin/env bash
# setup-vms.sh — correr DENTRO de cada VM
#
# 1. Activa sincronizacion de tiempo via NTP
# 2. Crea nodes.json con IPs de ejemplo — edita las IPs antes de iniciar el nodo
#
# Uso:
#   bash setup-vms.sh <host_id>
#
# Ejemplo en VM2:
#   bash setup-vms.sh 2

set -euo pipefail

if [[ $# -lt 1 ]]; then
  echo "Uso: $0 <host_id>"
  echo "Ejemplo: $0 2"
  exit 1
fi

HOST_ID=$1

# ── NTP ───────────────────────────────────────────────────────────────────────

echo "=== Activando NTP ==="
sudo timedatectl set-ntp true
timedatectl status | grep "NTP service" || true
echo "  ✓ NTP activo"

# ── directorios ───────────────────────────────────────────────────────────────

echo ""
echo "=== Creando directorios ==="
mkdir -p ~/node_messager/data ~/node_messager/logs ~/node_messager/messages ~/node_messager/tickets
echo "  ✓ directorios listos"

# ── nodes.json ────────────────────────────────────────────────────────────────

echo ""
echo "=== Creando nodes.json ==="
cat > ~/node_messager/nodes.json << EOF
{
  "master_id": 1,
  "host_id": ${HOST_ID},
  "nodes": [
    { "id": 1, "name": "sucursal1", "host": "192.168.100.102", "port": 5001 },
    { "id": 2, "name": "sucursal2", "host": "192.168.100.103", "port": 5001 },
    { "id": 3, "name": "sucursal3", "host": "192.168.100.104", "port": 5001 },
    { "id": 4, "name": "sucursal4", "host": "192.168.100.105", "port": 5001 }
  ]
}
EOF
echo "  ✓ nodes.json creado con host_id=${HOST_ID}"

# ── listo ─────────────────────────────────────────────────────────────────────

echo ""
echo "=== VM${HOST_ID} lista ==="
echo "Clona el repo en ~/node_messager y ejecuta:"
echo "  cd ~/node_messager && go run ./cmd"
