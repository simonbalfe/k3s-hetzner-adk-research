#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
env_file="$root/.env"
touch "$env_file"
chmod 600 "$env_file"

append_secret() {
  local name=$1
  if ! grep -q "^${name}=." "$env_file"; then
    printf '%s=%s\n' "$name" "$(openssl rand -hex 32)" >>"$env_file"
  fi
}

append_secret K3S_TOKEN
append_secret API_TOKEN
