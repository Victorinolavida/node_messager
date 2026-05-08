#!/usr/bin/env bash
# setup-vms.sh — genera configs y despliega a las VMs
#
# Uso:
#   ./setup-vms.sh <IP_VM1> <IP_VM2> <IP_VM3> <IP_VM4> [usuario]
#
# Ejemplos:
#   ./setup-vms.sh 192.168.100.10 192.168.100.11 192.168.100.12 192.168.100.13
#   ./setup-vms.sh 192.168.100.10 192.168.100.11 192.168.100.12 192.168.100.13 root
#   ./setup-vms.sh 192.168.100.10 192.168.100.11 192.168.100.12 192.168.100.13 --only-configs

set -euo pipefail

# ── argumentos ────────────────────────────────────────────────────────────────

if [[ $# -lt 4 ]]; then
  echo "Uso: $0 <IP_VM1> <IP_VM2> <IP_VM3> <IP_VM4> [usuario|--only-configs]"
  exit 1
fi

IP1=$1
IP2=$2
IP3=$3
IP4=$4
USER=${5:-""}
PORT=5001
MASTER_ID=1
BINARY="node_messager_linux_amd64"
REMOTE_DIR="~/node_messager"

# ── generar configs ───────────────────────────────────────────────────────────

generate_config() {
  local vm_id=$1
  local host_ip=$2
  local host_name="sucursal${vm_id}"
  local out="nodes-vm${vm_id}.json"

  cat > "$out" <<EOF
{
  "master_id": ${MASTER_ID},
  "nodes": [
    { "id": 1, "name": "sucursal1", "host": "${IP1}", "port": ${PORT} },
    { "id": 2, "name": "sucursal2", "host": "${IP2}", "port": ${PORT} },
    { "id": 3, "name": "sucursal3", "host": "${IP3}", "port": ${PORT} },
    { "id": 4, "name": "sucursal4", "host": "${IP4}", "port": ${PORT} }
  ],
  "host": { "id": ${vm_id}, "name": "${host_name}", "host": "0.0.0.0", "port": ${PORT} }
}
EOF
  echo "  ✓ generado: $out"
}

echo ""
echo "=== Generando configs ==="
generate_config 1 "$IP1"
generate_config 2 "$IP2"
generate_config 3 "$IP3"
generate_config 4 "$IP4"

# ── solo configs ──────────────────────────────────────────────────────────────

if [[ "$USER" == "--only-configs" || -z "$USER" ]]; then
  echo ""
  echo "Configs generados. Para desplegar:"
  echo "  $0 $IP1 $IP2 $IP3 $IP4 <usuario_ssh>"
  echo ""
  exit 0
fi

# ── compilar binario linux ────────────────────────────────────────────────────

echo ""
echo "=== Compilando binario Linux ==="
GOOS=linux GOARCH=amd64 go build -ldflags "-X main.debug=false" -o "$BINARY" ./cmd
echo "  ✓ $BINARY listo"

# ── desplegar a cada VM ───────────────────────────────────────────────────────

deploy() {
  local vm_id=$1
  local ip=$2
  local cfg="nodes-vm${vm_id}.json"
  local target="${USER}@${ip}"

  echo ""
  echo "--- Desplegando a VM${vm_id} ($ip) ---"

  # crear directorio remoto
  ssh "$target" "mkdir -p ${REMOTE_DIR}/data ${REMOTE_DIR}/logs ${REMOTE_DIR}/messages ${REMOTE_DIR}/tickets"

  # copiar binario y config
  scp "$BINARY" "${target}:${REMOTE_DIR}/node_messager"
  scp "$cfg"    "${target}:${REMOTE_DIR}/nodes.json"

  # hacer ejecutable
  ssh "$target" "chmod +x ${REMOTE_DIR}/node_messager"

  echo "  ✓ VM${vm_id} lista"
  echo "  → ssh ${target} 'cd ${REMOTE_DIR} && ./node_messager'"
}

echo ""
echo "=== Desplegando a VMs ==="
deploy 1 "$IP1"
deploy 2 "$IP2"
deploy 3 "$IP3"
deploy 4 "$IP4"

# ── resumen ───────────────────────────────────────────────────────────────────

echo ""
echo "=== Despliegue completo ==="
echo ""
echo "Para iniciar cada nodo (recomendado: iniciar VM1 primero):"
echo ""
echo "  VM1 (maestro): ssh ${USER}@${IP1} 'cd ${REMOTE_DIR} && ./node_messager'"
echo "  VM2:           ssh ${USER}@${IP2} 'cd ${REMOTE_DIR} && ./node_messager'"
echo "  VM3:           ssh ${USER}@${IP3} 'cd ${REMOTE_DIR} && ./node_messager'"
echo "  VM4:           ssh ${USER}@${IP4} 'cd ${REMOTE_DIR} && ./node_messager'"
echo ""
