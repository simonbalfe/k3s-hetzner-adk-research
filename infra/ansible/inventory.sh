#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
outputs=$(tofu -chdir="$root/infra/hetzner" output -json)
key=${SSH_PRIVATE_KEY_PATH:-$HOME/.ssh/id_ed25519}

jq -n \
  --arg control_name "$(jq -r '.control_plane.value.name' <<<"$outputs")" \
  --arg control_public "$(jq -r '.control_plane.value.public_ip' <<<"$outputs")" \
  --arg control_private "$(jq -r '.control_plane.value.private_ip' <<<"$outputs")" \
  --arg worker_name "$(jq -r '.system_worker.value.name' <<<"$outputs")" \
  --arg worker_public "$(jq -r '.system_worker.value.public_ip' <<<"$outputs")" \
  --arg worker_private "$(jq -r '.system_worker.value.private_ip' <<<"$outputs")" \
  --arg key "$key" \
  '{
    control_plane: {hosts: [$control_name]},
    system_workers: {hosts: [$worker_name]},
    _meta: {hostvars: {
      ($control_name): {ansible_host: $control_public, private_ip: $control_private},
      ($worker_name): {ansible_host: $worker_public, private_ip: $worker_private}
    }},
    all: {vars: {ansible_user: "root", ansible_ssh_private_key_file: $key}}
  }'
