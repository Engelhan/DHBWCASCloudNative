# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Keeping this file current

**Update this file whenever you make a significant change** — a new component, a reversed
architecture decision, a completed item from "Current implementation status", a changed
version pin, a new blocker. Small edits/bugfixes don't need an update. Prefer editing the
existing section over appending a new one; keep the "Current implementation status" section in
particular in sync with reality so it never drifts into a stale snapshot like a one-off status
dump would.

## What this repo is

Exam project ("Laborarbeit: Cloud-native Anwendung", DHBW) building **TicketFlow**, a
cloud-native event-ticket-sales platform, entirely on the DHBWCloud OpenStack platform (no
managed external cloud services allowed — no hosted DB, no hosted Kubernetes, no managed
Grafana, no external LLM APIs; only container registries/package sources and the git host are
permitted external dependencies). The full graded spec is in `2026_07_17_Aufgabenstellung.pdf`.

**The root `README.md` is itself a graded deliverable** with a fixed, mandatory section order
(`## Problem`, `## Architektur`, `## Betrieb`, `## Entscheidungen`, `## Grenzen`, optionally
`## Bonus`). Do not rename, reorder, or remove these H2 headings. `## Entscheidungen` requires
one paragraph per mandatory/chosen topic (P1–P7, plus the two chosen elective topics) with
*Wahl* / *Alternative* / *Begründung* — always tie the reasoning to the domain (flash-sale ticket
demand), not to lecture content, per the grading rubric.

Chosen elective topics (fixed, do not add a third without asking — a third only earns bonus
points and only if Part A is fully done): **W1 Packaging** (Helm chart, `values-dev.yaml` /
`values-prod.yaml` / values schema) and **W2 GitOps** (ArgoCD, App-of-Apps, auto-sync + self-heal).

## Domain model (why the infra exists)

Ticket sales for live events cause "flash sale" spikes: many concurrent reservation attempts for
a small, fixed seat count. This is the load path the whole architecture exists to handle —
`catalog-api` never writes reservations synchronously; it publishes an event and returns.

- `catalog-api` — public REST API, reads seat availability from Postgres, publishes
  `ReservationRequested` to Kafka.
- `reservation-worker` — Kafka consumer, does the actual transactional seat lock in
  CNPG-Postgres, publishes `ReservationConfirmed`/`ReservationRejected`. This is the service KEDA
  scales, on Kafka consumer-group lag (not CPU — lag is the metric that actually reflects the
  domain problem).
- `notification-worker` — second, independent Kafka consumer group off the same result topic
  (proves Kafka is used as a bus with fan-out, not just a point-to-point queue between two
  services).
- `frontend`, `load-generator` — demo UI and the k6 flash-sale load script.
- Stateful services: **CNPG** (PostgreSQL operator) and **Strimzi** (Kafka).

None of `src/*` has code yet — only per-service `README.md` files describing the planned
interface/config. When implementing a service, keep it consistent with its README (env vars via
ConfigMap/Secret, `/metrics`, `/healthz`, `/readyz`).

## Infra layering (read this before touching infra/)

Four layers, each owning a distinct concern — don't blur them:

1. **`infra/terraform/`** — OpenStack resources only (VMs + keypair). No Kubernetes, no app
   config here.
2. **`infra/ansible/`** — configures the VMs, installs k3s, labels nodes, bootstraps ArgoCD.
   Nothing beyond that.
3. **`gitops/`** (ArgoCD Applications) — everything Kubernetes-level from here on: operators
   (CNPG, Strimzi, KEDA, kube-prometheus-stack) and the app chart itself, as GitOps-synced
   `Application` resources. Currently only `.gitkeep` placeholders — **not implemented yet**.
4. **`charts/ticketflow/`** — the umbrella Helm chart for the app layer (services, CNPG
   `Cluster` CR, Strimzi `Kafka` CR, ServiceMonitors, KEDA `ScaledObject`). Currently only a
   README — **no templates written yet**.

Terraform → Ansible → ArgoCD → Helm chart is a hard boundary an agent should preserve: e.g. don't
add k3s app deployments via Ansible, don't add OpenStack resources via a Helm hook, etc.

### Terraform specifics (implemented, tested by the user)

- No network/subnet/router/security-group management — the DHBWCloud project already provides an
  existing, externally routable network named **`DHBWV6`**, looked up via a data source
  (`network_name` var, default `"DHBWV6"`). VMs attach to it directly.
- **`DHBWV6` is IPv6-only.** VMs get no IPv4 address at all — use `access_ip_v6`, never
  `access_ip_v4`, anywhere in `modules/compute`. Any new resource/output touching instance IPs
  must follow this.
- No security groups are created or managed here (explicit user decision) — the project's
  default security group applies.
- Auth is via **OpenStack Application Credentials** (`os_application_credential_id` /
  `os_application_credential_secret`), not username/password.
- Placement: there is no "storage-optimized node" concept. Instead every worker gets a
  **virtually simulated region** label via instance metadata (`worker_regions`, default
  `["region-a", "region-b", "region-c"]`, one worker per entry) — DHBWCloud only has one real
  region/AZ, so this is a deliberately fake topology dimension used later for P4 (CNPG/Kafka
  topology-spread across simulated regions, so no single node/region loss takes out all
  replicas). Don't reintroduce a real `availability_zone` OpenStack argument for this.
- Terraform **generates `infra/ansible/inventory/hosts.ini` itself** (`local_file` +
  `templates/hosts.ini.tpl`) from the compute module outputs — this is intentional automation for
  reproducibility (P2: "ein Ablauf bringt alles hoch"). Don't hand-edit `hosts.ini`; edit the
  `.tpl` and re-`apply`. Any module/module-output rename must be reflected in the template.
  `infra/ansible/inventory/hosts.ini.example` is a static, non-generated fallback for reference
  only.
- Only one Terraform module remains: `modules/compute`. `modules/network` was deliberately
  deleted (superseded by the `DHBWV6` lookup) — don't recreate it.
- Every module that uses a provider directly needs its **own** `required_providers` block with
  the full source address (see `modules/compute/versions.tf`) — Terraform does not infer a
  non-default-namespace provider source from the root module for child modules. Omitting this
  produces a `does not have a provider named registry.terraform.io/hashicorp/openstack` error.
- Pinned versions (bump deliberately, not casually): `terraform-provider-openstack/openstack ~> 3.4`,
  `hashicorp/local ~> 2.9`. `terraform.tfvars` is gitignored; `terraform.tfvars.example` is the
  template to copy.

### Ansible specifics (verified in WSL against real DHBWCloud VMs)

- Roles run in a fixed order (see `playbooks/site.yml`): `common` → `k3s_server` → `k3s_agent` →
  `node_labels` → `argocd_bootstrap`. Each depends on state from the previous one (e.g. agents
  need the server's node-token; labels need registered nodes).
- k3s is installed with **IPv6-specific flags** because the cluster network is IPv6-only:
  `--node-ip`/`--tls-san` on the node's own IPv6, `--cluster-cidr`/`--service-cidr` in the ULA
  range `fd00::/8` (not globally routable on purpose), `--flannel-ipv6-masq` for pod egress NAT.
  Versions/CIDRs are centralized in `inventory/group_vars/all.yml` — change them there, not inline
  in role tasks. It lives under `inventory/`, not directly under `infra/ansible/`, because Ansible
  only auto-loads `group_vars/` when it sits next to the inventory file (or next to the playbook) —
  a plain top-level `infra/ansible/group_vars/` is silently never loaded and every pinned var
  resolves to undefined. Verified by reproducing the documented `ansible-playbook playbooks/site.yml`
  invocation with a debug task.
- `k3s_server` fetches the kubeconfig locally and rewrites the server URL from `127.0.0.1` to
  `https://[<ipv6>]:6443` (bracket syntax — required for IPv6 host:port, easy to get wrong).
- `common` sets each node's OS hostname to its Terraform instance name so the k3s node name is
  deterministic — `node_labels` and the agent-join readiness check both rely on
  `inventory_hostname` matching the actual Kubernetes node name.
- `command:` given as a plain string is shlex-parsed by Ansible before exec, which silently
  strips unescaped `"` — the agent-join readiness check's
  `jsonpath={.status.conditions[?(@.type=="Ready")].status}` lost its quotes around `Ready` this
  way (`kubectl` then errored with "unrecognized identifier Ready"), even though the join itself
  had already succeeded. Fixed by switching that task to `command: {argv: [...]}` (a real list,
  each element passed to the process as-is, no shlex). Any future `command`/`shell` string
  containing embedded `"` needs the same treatment.
- Real-VM run also confirmed: k3s server/agent join, IPv6 node registration (`kubectl get nodes
  -o wide` shows the real DHBWV6 addresses), and node Ready state all work as designed.
- `argocd_bootstrap` installs a version-pinned ArgoCD **core** manifest (not HA — single
  control-plane lab cluster) and applies `gitops/apps/app-of-apps.yaml` **only if that file
  already exists** (checked via `stat`, skips otherwise instead of failing) — expected to still
  be a no-op until the GitOps layer is written.
- Pinned versions in `inventory/group_vars/all.yml`: k3s `v1.36.2+k3s1`, ArgoCD `v3.4.4` (latest
  stable, not the 3.5 release candidates at time of writing).
- **Ansible cannot run as the controller on native Windows** (`ansible-core` fails with
  `check_blocking_io ... WinError 87` — a known Windows limitation; Windows is only supported as
  a managed WinRM target, never as the control node). Run `ansible-playbook` from WSL or a
  Linux/macOS shell, not from PowerShell/Git Bash on Windows directly, even inside a venv.
- Ansible itself runs from a Python venv at the repo root (`.venv/`, gitignored) — the WSL image
  has no system `pip`/`venv` and no passwordless sudo to install them via apt, so a venv (bootstrap
  via `get-pip.py` if needed) is the path of least friction. Pinned in
  `infra/ansible/requirements.txt` (`ansible==14.2.0`, `ansible-lint==26.6.0`), same
  no-latest-tags principle as k3s/ArgoCD. See `infra/ansible/README.md` ## Voraussetzungen.
- `ansible-playbook --syntax-check` on both playbooks, `ansible-lint`, `yamllint`, and a debug-task
  render check of every Jinja expression (install commands, join URL, node-label loop, kubeconfig
  regex) against the example inventory all pass. Two real bugs found and fixed this way: (1)
  `group_vars/all.yml` lived directly under `infra/ansible/`, a location Ansible never auto-loads
  (see above — moved to `inventory/group_vars/`), so every pinned version/CIDR silently resolved to
  undefined and the `k3s-Server installieren` task would have failed immediately on a real run; (2)
  the `curl | sh` k3s install in `k3s_server`/`k3s_agent` had no `pipefail`, so a failed download
  could report success (`sh` exits 0 on empty stdin) — added `set -o pipefail`. What's still
  unverified: actual apt/hostname/k3s/ArgoCD behavior against real VMs (needs `terraform apply`
  first), and the passwordless-sudo/cloud-image assumptions in `infra/ansible/README.md`
  ## Offene Punkte.

## Commands

```bash
# Terraform (infra/terraform/)
cd infra/terraform
cp terraform.tfvars.example terraform.tfvars   # fill in OpenStack app credential + flavors/image
terraform init
terraform plan
terraform apply        # also generates ../ansible/inventory/hosts.ini

# Ansible (infra/ansible/) — must run from WSL/Linux, not native Windows
source .venv/bin/activate   # repo-root venv, see infra/ansible/README.md ## Voraussetzungen
cd infra/ansible
ansible-playbook playbooks/site.yml
ansible-playbook playbooks/bootstrap-gitops.yml   # re-run only the ArgoCD/App-of-Apps step
export KUBECONFIG=infra/ansible/kubeconfig
kubectl get nodes -o wide
kubectl -n argocd get pods

# Ansible static checks (no VMs needed)
cd infra/ansible
ansible-playbook playbooks/site.yml --syntax-check
ansible-lint
```

No app build/lint/test commands exist yet — `src/*` has no code, and `charts/ticketflow` has no
templates to `helm lint`/`helm template` against. Add real commands here once those exist instead
of leaving this section stale.

## Current implementation status (update this section as work lands)

- **Done:** `infra/terraform/` (tested by user), `infra/ansible/` (written, verified both
  statically and against real DHBWCloud VMs — k3s server/agent join and IPv6 node registration
  confirmed working; three real bugs found+fixed along the way, see Ansible specifics above), repo
  scaffold + all folder-level READMEs, root
  `README.md` skeleton with all required sections (many still marked `TODO` pending the pieces
  below).
- **Not started:** `gitops/apps/*.yaml` (App-of-Apps + per-operator Applications),
  `charts/ticketflow/templates/*`, all `src/*` application code, `observability/dashboards/` +
  `observability/alerts/` content.
- Commit history currently has a single large commit — the assignment explicitly penalizes
  "Monolith-Commits"; keep new work in small, separately-committed steps from here on.
