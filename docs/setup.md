# Setup

## Requirements

OpenTofu, Ansible, Helm, kubectl, jq, SSH, and a Codex CLI signed in with ChatGPT.

Use a dedicated Hetzner project and its read/write API token. The token determines which project receives resources.

## Provision

```bash
cp .env.example .env
chmod 600 .env
./scripts/init-secrets.sh
cp infra/hetzner/terraform.tfvars.example infra/hetzner/terraform.tfvars
```

Set `HCLOUD_TOKEN` in `.env`. Set your SSH public-key path and trusted public IP CIDR in `terraform.tfvars`.

```bash
make plan
make apply
make bootstrap
make platform
./scripts/publish-image.sh demo-1
./scripts/deploy.sh demo-1
```

`make apply` creates billable resources. SSH must trust the new hosts before Ansible runs; verify their fingerprints through your trusted Hetzner access path.

## Codex login

`make platform` seeds your file-backed ChatGPT login into an encrypted Kubernetes Secret. The worker runs the official Codex CLI, which refreshes its own login and saves the updated file back to the Secret after each job. One worker runs at a time for this account. Credentials stay out of images and Git.

If your login expires or changes:

```bash
codex login
./scripts/sync-codex-auth.sh --replace
export KUBECONFIG="$PWD/.state/kubeconfig"
kubectl -n research rollout restart deployment/research-worker
```

Use `CODEX_AUTH_FILE` to select another file-backed ChatGPT login. The original local login is only used for initial seeding or an explicit `--replace`; routine platform upgrades preserve the refreshed cluster copy.

The initial copy and your local Codex login share a session. If both environments refresh it independently, one copy may stop working. A dedicated ChatGPT login session for the cluster avoids that conflict; reseed with `--replace` if Codex returns an authentication error.

## Operate

```bash
export KUBECONFIG="$PWD/.state/kubeconfig"
kubectl get nodes
kubectl -n research get pods,scaledobjects
kubectl -n research logs deployment/research-worker
kubectl -n kube-system logs deployment/cluster-autoscaler-hetzner-cluster-autoscaler
```

Use `kubectl port-forward` for API access. Public HTTPS and DNS are not configured.

## Delivery

Run `publish-image.sh TAG`, then `deploy.sh TAG`. Helm waits for a healthy rollout and rolls back a failed release.

GitHub workflows provide CI and optional release/deployment templates. They need a GitHub repository and a configured self-hosted runner. Local delivery does not depend on GitHub.

Place the runner on the system VM with the `adk-research` label, Docker access, Helm, and a deployment kubeconfig. A `v*` tag builds into the private registry; the manual Deploy workflow selects that tag. Override the `IMAGE_REPOSITORY` repository variable if the system VM's private IP changes.

## State and cleanup

Keep `.env`, `.state/kubeconfig`, and OpenTofu state private. Back up the state before managing this cluster from another machine.

Before removing the cluster, stop accepting jobs, drain the queue, and let elastic nodes scale to zero. Then remove platform releases and run `tofu destroy` from `infra/hetzner` with `HCLOUD_TOKEN` exported. OpenTofu does not own autoscaler-created VMs; verify those are gone in Hetzner.
