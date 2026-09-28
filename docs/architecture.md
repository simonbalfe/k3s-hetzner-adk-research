# Architecture

```mermaid
flowchart TB
  T[OpenTofu] --> C[Control-plane VM]
  T --> S[System VM]
  A[Ansible] --> C
  A --> S
  subgraph k3s
    C --- CA[Cluster Autoscaler]
    S --- API[Go API]
    S --- R[Redis]
    S --- K[KEDA]
    S --- Registry[Private image registry]
    W[Elastic worker VM] --- P[Eino worker pod]
  end
  K -->|queue backlog| P
  CA -->|pending pods| W
  P --> Codex[Codex CLI: web research]
  P --> Auth[Codex login Secret]
```

| Component | Owns |
|---|---|
| OpenTofu | Network, firewall, SSH key, two baseline VMs |
| Ansible | k3s installation and configuration |
| Hetzner cloud controller | Kubernetes node identity and networking integration |
| Cluster Autoscaler | Zero to one elastic worker VM |
| KEDA | Zero to one worker pod |
| Go worker | One Eino agent run at a time |

## A job

1. The API authenticates the request, stores a job, and returns its ID.
2. Redis Streams queues the job.
3. KEDA requests worker pods. Cluster Autoscaler creates a VM if needed.
4. The Go worker runs an Eino chat agent whose model adapter calls `codex exec` with web search.
5. The worker stores the report in Redis. The client polls the API for it.
6. After idle cooldowns, worker pods and elastic VMs are removed.

## Isolation

System services stay on the fixed system VM. Labels and taints reserve elastic VMs for research. Each Codex run gets a temporary workspace and a read-only command sandbox, with shell tools disabled.

The worker mounts a writable copy of your file-backed Codex login. Codex refreshes it when needed; the worker saves the new copy to an encrypted Kubernetes Secret for the next pod. Its service account can patch only that Secret. A single active worker avoids concurrent refreshes of the same login.

SSH and the Kubernetes API are restricted to operator CIDRs. Redis and the image registry are private. Baseline VMs remain running when there are no jobs.

## Playground limits

- One Codex subscription login supports one active worker in this setup.
- Redis uses temporary storage; replacing its pod loses jobs and reports.
- A worker crash can leave a job pending; resubmit it in this minimal demo.
- No public ingress or high availability.

The directory and Go module retain the original `adk-research` name; the runtime uses Codex.
