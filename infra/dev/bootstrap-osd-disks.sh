#!/usr/bin/env bash
# infra/dev/bootstrap-osd-loopback.sh — bootstrap OSD storage on Lima nodes.
#
# Approach: each Lima VM gets a 30 GiB additional virtio disk (qcow2-backed)
# that appears as /dev/vdb inside the guest. This is what
# infra/dev/ceph-single-node.yaml expects. Loopback files were tried first
# but containerd does not export them to OSD-prepare pods reliably.
#
# This script:
#   1. Creates 3 lima disks (rook-osd-master, rook-osd-node1, rook-osd-node2)
#      if they do not already exist.
#   2. Stops each VM, appends `additionalDisks:` to its lima.yaml, restarts.
#   3. Confirms /dev/vdb is visible inside each VM.
#
# Idempotent: re-runs are safe — both `limactl disk create` and the YAML
# patch detect the existing state.
#
# Producción usa block devices reales en cada nodo; este script existe SOLO
# para que dev/Lima reproduzca la topología (same Rook-Ceph operator, same
# Ceph image, same CRDs) sin recrear las VMs desde cero.
set -euo pipefail

declare -A DISKS=(
  [k3s-master]=rook-osd-master
  [k3s-node1]=rook-osd-node1
  [k3s-node2]=rook-osd-node2
)

# 1. create disks
for vm in "${!DISKS[@]}"; do
  disk="${DISKS[$vm]}"
  if limactl disk list 2>/dev/null | awk 'NR>1 {print $1}' | grep -qx "$disk"; then
    echo "[$vm] disk $disk already exists"
  else
    echo "[$vm] creating disk $disk (30GiB)"
    limactl disk create "$disk" --size 30GiB
  fi
done

# 2. patch each VM yaml + restart
for vm in "${!DISKS[@]}"; do
  disk="${DISKS[$vm]}"
  yaml="$HOME/.lima/$vm/lima.yaml"
  if grep -q "name: \"$disk\"" "$yaml"; then
    echo "[$vm] additionalDisks already wired"
  else
    echo "[$vm] adding additionalDisks: $disk + restart"
    limactl stop "$vm" >/dev/null 2>&1 || true
    cat >> "$yaml" <<EOF

additionalDisks:
  - name: "$disk"
EOF
    limactl start "$vm"
  fi
done

# 3. confirm /dev/vdb visible
for vm in "${!DISKS[@]}"; do
  echo "=== $vm ==="
  limactl shell "$vm" -- sudo lsblk -d -o NAME,SIZE,TYPE | head -8
done
