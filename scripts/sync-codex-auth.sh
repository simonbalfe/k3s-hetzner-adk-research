#!/usr/bin/env bash
set -euo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
if [[ -f "$root/.env" ]]; then
  set -a
  source "$root/.env"
  set +a
fi
export KUBECONFIG=${KUBECONFIG:-$root/.state/kubeconfig}
auth_file=${CODEX_AUTH_FILE:-$HOME/.codex/auth.json}
jq -e '.auth_mode == "chatgpt" and .tokens != null' "$auth_file" >/dev/null
kubectl -n research create secret generic codex-auth \
  --from-file=auth.json="$auth_file" \
  --dry-run=client -o yaml | kubectl apply -f -
