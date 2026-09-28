#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
set -a
source "$root/.env"
set +a

registry_ip=$(tofu -chdir="$root/infra/hetzner" output -json system_worker | jq -r .private_ip)
IMAGE_REPOSITORY=${IMAGE_REPOSITORY:-$registry_ip:30500/research-agents}
IMAGE_TAG=${1:-${IMAGE_TAG:?IMAGE_TAG or a tag argument is required}}

export KUBECONFIG=${KUBECONFIG:-$root/.state/kubeconfig}

helm upgrade --install research-agents "$root/deploy/chart" \
  --namespace research \
  --set image.repository="$IMAGE_REPOSITORY" \
  --set image.tag="$IMAGE_TAG" \
  --atomic \
  --wait
