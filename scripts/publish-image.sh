#!/usr/bin/env bash
set -euo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
set -a
source "$root/.env"
set +a
ssh_options=(-i "${SSH_PRIVATE_KEY_PATH:-$HOME/.ssh/id_ed25519}" -o BatchMode=yes)
tag=${1:?pass an immutable image tag}
[[ "$tag" =~ ^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,127}$ ]] || exit 1
worker=$(tofu -chdir="$root/infra/hetzner" output -json system_worker | jq -r .public_ip)
remote_dir=$(ssh "${ssh_options[@]}" "root@$worker" mktemp -d)
[[ "$remote_dir" =~ ^/tmp/tmp\.[a-zA-Z0-9]+$ ]] || exit 1
trap 'ssh "${ssh_options[@]}" "root@$worker" "rm -rf -- $remote_dir"' EXIT
COPYFILE_DISABLE=1 tar -C "$root" -czf - Dockerfile go.mod go.sum cmd internal | ssh "${ssh_options[@]}" "root@$worker" "tar -xzf - -C $remote_dir"
ssh "${ssh_options[@]}" "root@$worker" "docker build -t localhost:30500/research-agents:$tag $remote_dir && docker push localhost:30500/research-agents:$tag"
