#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
set -a
source "$root/.env"
set +a

: "${HCLOUD_TOKEN:?HCLOUD_TOKEN is required}"

tofu -chdir="$root/infra/hetzner" init
tofu -chdir="$root/infra/hetzner" plan
