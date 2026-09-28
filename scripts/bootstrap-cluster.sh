#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
set -a
source "$root/.env"
set +a

: "${K3S_TOKEN:?K3S_TOKEN is required}"
mkdir -p "$root/.state"
chmod 700 "$root/.state"

cd "$root/infra/ansible"
ansible-playbook site.yml
