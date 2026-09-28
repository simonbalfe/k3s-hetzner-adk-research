#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
set -a
source "$root/.env"
set +a

: "${HCLOUD_TOKEN:?HCLOUD_TOKEN is required}"
: "${K3S_TOKEN:?K3S_TOKEN is required}"
: "${API_TOKEN:?API_TOKEN is required}"

export KUBECONFIG="$root/.state/kubeconfig"
network_id=$(tofu -chdir="$root/infra/hetzner" output -raw network_id)
firewall_id=$(tofu -chdir="$root/infra/hetzner" output -raw firewall_id)
ssh_key_id=$(tofu -chdir="$root/infra/hetzner" output -raw ssh_key_id)
location=$(tofu -chdir="$root/infra/hetzner" output -raw location)
control_ip=$(tofu -chdir="$root/infra/hetzner" output -json control_plane | jq -r .private_ip)
registry_ip=$(tofu -chdir="$root/infra/hetzner" output -json system_worker | jq -r .private_ip)

kubectl create namespace research --dry-run=client -o yaml | kubectl apply -f -
kubectl -n kube-system create secret generic hcloud \
  --from-literal=token="$HCLOUD_TOKEN" \
  --from-literal=network="$network_id" \
  --dry-run=client -o yaml | kubectl apply -f -

cloud_init=$(sed -e "s/__CONTROL_PRIVATE_IP__/$control_ip/g" -e "s/__K3S_TOKEN__/$K3S_TOKEN/g" -e "s/__REGISTRY_ADDRESS__/$registry_ip:30500/g" "$root/deploy/platform/autoscaler-cloud-init.yaml.tmpl")
cluster_config=$(jq -n \
  --arg cloud_init "$cloud_init" \
  '{imagesForArch:{amd64:"ubuntu-24.04"},defaultSubnetIPRange:"10.20.0.0/24",nodeConfigs:{"research-agents":{cloudInit:$cloud_init,labels:{workload:"agents"},serverLabels:{cluster:"adk-research",role:"agent"},taints:[{key:"workload",value:"agents",effect:"NoSchedule"}]}}}')
cluster_config_base64=$(printf '%s' "$cluster_config" | base64 | tr -d '\n')

kubectl -n kube-system create secret generic cluster-autoscaler-hetzner \
  --from-literal=HCLOUD_TOKEN="$HCLOUD_TOKEN" \
  --from-literal=HCLOUD_CLUSTER_CONFIG="$cluster_config_base64" \
  --from-literal=HCLOUD_NETWORK="$network_id" \
  --from-literal=HCLOUD_FIREWALL="$firewall_id" \
  --from-literal=HCLOUD_SSH_KEY="$ssh_key_id" \
  --from-literal=HCLOUD_PUBLIC_IPV4=true \
  --from-literal=HCLOUD_PUBLIC_IPV6=true \
  --dry-run=client -o yaml | kubectl apply -f -

helm repo add hcloud https://charts.hetzner.cloud
helm repo add autoscaler https://kubernetes.github.io/autoscaler
helm repo add kedacore https://kedacore.github.io/charts
helm repo update

helm upgrade --install hccm hcloud/hcloud-cloud-controller-manager \
  --namespace kube-system \
  --version 1.35.0 \
  --values "$root/deploy/platform/hccm-values.yaml" \
  --wait

helm upgrade --install cluster-autoscaler autoscaler/cluster-autoscaler \
  --namespace kube-system \
  --version 9.59.0 \
  --values "$root/deploy/platform/autoscaler-values.yaml" \
  --set "autoscalingGroups[0].region=$location" \
  --wait

helm upgrade --install keda kedacore/keda \
  --namespace keda \
  --create-namespace \
  --version 2.21.0 \
  --wait

kubectl apply -f "$root/deploy/platform/traefik-config.yaml"
kubectl apply -f "$root/deploy/demo/redis.yaml"
kubectl apply -f "$root/deploy/demo/registry.yaml"
kubectl -n research create secret generic research-secrets \
  --from-literal=api-token="$API_TOKEN" \
  --dry-run=client -o yaml | kubectl apply -f -

"$root/scripts/sync-codex-auth.sh"
