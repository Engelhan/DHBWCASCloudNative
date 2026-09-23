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
`values-prod.yaml` / values schema) and **W2 GitOps** (ArgoCD, one `ApplicationSet`, auto-sync + self-heal).

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

`catalog-api`, `frontend`, `notification-worker`, `reservation-worker` and `load-generator` all
have working code now (see Current implementation status below). When implementing a service,
keep it consistent with its README (env vars via ConfigMap/Secret, `/metrics`, `/healthz`,
`/readyz`).

## Infra layering (read this before touching infra/)

Four layers, each owning a distinct concern — don't blur them:

1. **`infra/terraform/`** — OpenStack resources only (VMs + keypair). No Kubernetes, no app
   config here.
2. **`infra/ansible/`** — configures the VMs, installs k3s, labels nodes, bootstraps ArgoCD.
   Nothing beyond that.
3. **`gitops/`** — everything Kubernetes-level from here on: operators (CNPG, Strimzi, KEDA), the
   observability stack (kube-prometheus-stack, Loki, Tempo), cluster addons (Longhorn,
   cert-manager, external-dns) and the app charts, all as GitOps-synced `Application` resources.
   **The folder holds exactly two files**: `env/cluster.yaml` and `applicationset.yaml`.
   - **`gitops/env/cluster.yaml` is the single source of truth** for every deployment-specific
     value (fork `repoURL`, `targetRevision`, `environment` dev/prod, `dnsZone`, `acmeEmail`,
     `acmeServer`, `rfc2136.{server,keyName}`, `imageRegistry`, `imageTag`, `namePrefix`), all
     under one top-level `ticketflow:` key — deliberately NOT `global:`, which Helm propagates
     into subcharts where upstream charts give it their own meaning. It is read by ArgoCD (git-files
     generator + multi-source `$values`), by every chart as `.Values.ticketflow.*`, by Ansible
     (`include_vars` in `gitops_secrets`) and by Terraform (`yamldecode` in `main.tf`). **Never
     reintroduce any of these values as a literal in a chart or role.**
   - **`gitops/applicationset.yaml` replaced the former 19 `gitops/apps/*.yaml` files** (plus
     `app-of-apps.yaml`). Matrix generator: git-files over `env/cluster.yaml` × a list of the 19
     apps. The list field is `chartPath`, **not** `path` — the git-files generator already
     occupies `.path` with file metadata and the two would collide in the cross product.
     `spec.template` is a typed Kubernetes object, so Go expressions may only appear inside
     string values; everything conditional (sync-wave annotation, `ServerSideApply`,
     `ignoreDifferences`) lives in `spec.templatePatch` at the bottom, which is a real text
     template. Element names must stay byte-identical to the old Application names — that is what
     lets the ApplicationSet adopt existing Applications instead of duplicating them.
   - The **one literal repo URL left in the whole repo** is the `repoURL` of that git-files
     generator (chicken-and-egg: it must know which repo to read `env/cluster.yaml` from). A fork
     therefore edits two things: `env/cluster.yaml` and that one line.
   - Every chart additionally gets `values-<environment>.yaml` with `ignoreMissingValueFiles:
     true`, so a chart that later gains a dev/prod split needs no ApplicationSet change.
   Every `Application` points at a chart under `charts/` in this repo (never directly at an
   upstream Helm repo) — for addons that exist upstream as a chart, that's a small wrapper chart
   whose `Chart.yaml` declares the upstream chart as a `dependencies` entry, with our values in
   its own `values.yaml` (see `charts/README.md`); ArgoCD resolves the dependency via
   `helm dependency build` at sync time. Longhorn/cert-manager/cert-manager-issuer/external-dns/
   argocd-ingress/traefik-config/cnpg-operator/strimzi-operator/kube-prometheus-stack/loki/tempo/
   keda/postgres-cluster/kafka-cluster/apicurio/akhq/ticketflow-app/reservation-worker/
   notification-worker/external-secrets-operator/openbao-postgres/openbao/openbao-secretstore
   are written (see `charts/keda`/the `keda` ApplicationSet element and the `keda-scaledobject.yaml`
   template in `charts/reservation-worker` under Current implementation status below — not yet
   live-verified, `helm dependency update` for `charts/keda` itself is blocked locally by this
   machine's TLS-interception proxy, same class of gap as the reservation-worker/notification-
   worker/catalog-api docker-compose block noted elsewhere in this file). CNPG/Strimzi/
   kube-prometheus-stack/KEDA all bundle CRDs over the 262144-byte annotation limit (CNPG's
   `poolers` CRD >650KB, Strimzi's `Kafka` CRD >330KB, kube-prometheus-stack's `Prometheus` CRD
   >800KB, KEDA's `scaledjobs` CRD >650KB) — their Applications need
   `syncOptions: [ServerSideApply=true]`, same fix as the ArgoCD install itself (see Ansible
   specifics below). `external-secrets-operator`'s `secretstores`/`clustersecretstores` CRDs
   measured ~252KB raw YAML each (chart 2.11.0) — close enough to the limit, and client-side
   apply's JSON-encoded `last-applied-configuration` annotation can exceed the raw YAML size, that
   it gets `ServerSideApply=true` too rather than verifying the exact margin. Loki/Tempo ship no
   CRDs, don't need it; neither does `openbao` (verified: zero CRDs in the upstream chart) or
   `openbao-postgres`/`openbao-secretstore` (both fully-custom, single-CR charts, same class as
   `postgres-cluster`). Observability persistence goes
   through Longhorn
   throughout (already the default StorageClass); Loki runs `SingleBinary` mode with a bundled
   MinIO as its S3-compatible chunk store, Tempo uses its local filesystem backend — both are
   PVC-backed. Grafana (from kube-prometheus-stack) gets Loki + Tempo wired in as
   `additionalDataSources`, one Grafana for the full LGTM stack. See `gitops/README.md` ##
   Observability-Stack.
   See
   `gitops/README.md` for the DNS/ACME activation flow.
4. **`charts/kafka-cluster/`** — Kafka + schema-registry chart (Carlos' scope). `templates/messaging/`
   (Strimzi `Kafka` CR in KRaft mode plus separate `broker`/`controller` `KafkaNodePool`s — not
   combined-role, matching a real Strimzi setup the user provided as reference — two
   `KafkaTopic`s, and a JMX-Prometheus-Exporter `ConfigMap` for P7; namespace `ticketflow`) plus
   `templates/schema-registry/` (registers `files/schemas/*.schema.json` into Apicurio) is written
   and deployed via the `kafka-cluster` ApplicationSet element (merged to `main`). **`catalog-api`+`frontend`
   are deliberately NOT part of this chart** — team decision, they must not be deployed in the
   same Application as Kafka — they live in their own chart, `charts/ticketflow-app/`, deployed
   via the `ticketflow-app` ApplicationSet element. Both charts share the `ticketflow` Kubernetes namespace
   so `catalog-api` can reach Kafka's bootstrap service by short in-namespace DNS name. KEDA's
   `ScaledObject`/CNPG's `Cluster` CR are not part of either chart — each lives with the workload
   it targets instead (KEDA's `ScaledObject`/`TriggerAuthentication` in `charts/reservation-worker`,
   analogous to how the Postgres `Cluster` CR lives in its own `charts/postgres-cluster`, not in
   `charts/cnpg-operator`/`charts/keda`, the operator-only wrapper charts).

Terraform → Ansible → ArgoCD → Helm chart is a hard boundary an agent should preserve: e.g. don't
add k3s app deployments via Ansible, don't add OpenStack resources via a Helm hook, etc.

**This boundary was briefly crossed and then reverted.** Branch `ansible-bootstrap` (commit
`938a66f`) ports cluster addons (Longhorn, cert-manager, external-dns, auto-upgrade,
connectivity-check preflight) from
[pfisterer/k3s-dhbw-cloud-role](https://github.com/pfisterer/k3s-dhbw-cloud-role) as Ansible
roles that call `kubernetes.core.helm` directly — a deliberate exception at the time. That
approach was superseded on `main` by deploying the same addons (Longhorn, cert-manager,
external-dns) through the GitOps layer instead (see `gitops/README.md`), restoring the boundary
above. **Don't merge `ansible-bootstrap` into `main`** — it would install the same Helm releases
a second time via a different tool and fight ArgoCD's `selfHeal`. `system-upgrade-controller`
(k3s auto-upgrade) has no equivalent in `gitops/` yet — deliberately skipped for now since
`k3s_auto_upgrade_enabled` defaulted to `false` anyway; port it as an ArgoCD `Application` later
if needed rather than reviving the Ansible role.

### Terraform specifics (implemented, tested by the user)

- No network/subnet/router/security-group management — the DHBWCloud project already provides an
  existing, externally routable network named **`DHBWV6`**, looked up via a data source
  (`network_name` var, default `"DHBWV6"`). VMs attach to it directly.
- **`DHBWV6` is dual-stack, not IPv6-only** (corrected after a live cluster showed this — VMs get
  a real, internet-routed IPv4 via DHCP on the same NIC in addition to their IPv6 address). IPv6
  stays the **primary** address everywhere (`access_ip_v6`, SSH, kubeconfig, k3s `--node-ip`
  first entry) — `access_ip_v4` (`modules/compute/outputs.tf`) is exposed *only* so k3s can run
  dual-stack (`--node-ip`/`--cluster-cidr`/`--service-cidr` each get an IPv6 value first, IPv4
  second — see Ansible specifics below), because pods otherwise cannot reach IPv4-only hosts
  like `github.com` (no `AAAA` record). Don't make IPv4 primary anywhere; it's additive only.
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
  `node_labels` → `argocd_bootstrap` → `gitops_secrets`. Each depends on state from the previous
  one (e.g. agents need the server's node-token; labels need registered nodes; `gitops_secrets`
  needs the kubeconfig `k3s_server` fetched locally).
- k3s runs **dual-stack, IPv6-primary**: `--node-ip`/`--cluster-cidr`/`--service-cidr` each take
  a comma-separated `<IPv6>,<IPv4>` pair (IPv6 first — kubeconfig/SSH stay IPv6-only; `--tls-san`
  gets both addresses as two separate flags, harmless and future-proofs the cert in case anything
  ever reaches the API server over IPv4), sourced from `ansible_host`/`node_ip_v4` (`node_ip_v4`
  comes from Terraform's `hosts.ini.tpl`, see Terraform specifics above) and `k3s_cluster_cidr(_v4)`/
  `k3s_service_cidr(_v4)` in `inventory/group_vars/all.yml`. IPv6 CIDRs stay in the ULA range
  `fd00::/8` (not globally routable on purpose); IPv4 CIDRs are k3s's own defaults
  (`10.42.0.0/16`/`10.43.0.0/16`, chosen to not collide with the VMs' own `10.200.0.0/19` DHCP
  range). `--flannel-ipv6-masq` handles IPv6 pod egress NAT (ULA isn't globally routable); IPv4
  pod egress is masqueraded by k3s/Flannel automatically, no equivalent flag needed. The whole
  IPv4 side exists for exactly one reason: pods (ArgoCD, external-dns) need to reach IPv4-only
  hosts like `github.com`, which has no `AAAA` record — see gitops layer above and
  `gitops/README.md` ## Privates Repo. Versions/CIDRs are centralized in
  `inventory/group_vars/all.yml` — change them there, not inline in role tasks. It lives under
  `inventory/`, not directly under `infra/ansible/`, because Ansible
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
  control-plane lab cluster) and applies `gitops/applicationset.yaml` **only if that file
  already exists** (checked via `stat`, skips otherwise instead of failing); the `stat` guard
  stays as a safety net for older checkouts/tags without it. This is the only object Ansible
  applies in the GitOps layer — ArgoCD creates the 19 Applications from it.
- `gitops_secrets` (new role) creates two things GitOps structurally can't provide itself: the
  ArgoCD repository credential for this repo (**this repo is private** — every generated
  `Application` uses `repoURL: git@github.com:...` SSH, not `https://`, since every one of them
  points at a chart under `charts/` in this repo, not directly at an upstream Helm repo — see
  Infra layering above; only the upstream Helm repos referenced from within each wrapper chart's
  `Chart.yaml.dependencies` need no credential, ArgoCD builds those via `helm dependency build`)
  and the RFC2136-TSIG `Secret`s that `charts/cert-manager-config`/`charts/external-dns`
  reference via `secretKeyRef`. **The repo URL is no longer hardcoded here** — the role reads it
  via `include_vars` from `gitops/env/cluster.yaml`. It used to be a literal, and was the one
  place a fork reliably missed: the credential got registered for the *original* repo and every
  Application stayed `Unknown` with no error pointing at the cause. This is a narrow, deliberate exception to "Ansible does nothing beyond
  k3s/labels/ArgoCD", **not** a repeat of the reverted `ansible-bootstrap` approach: it creates
  zero Helm releases/operators (those stay 100% ArgoCD-owned), only `Secret` objects containing
  credential material that, by design, must never be committed to git and so can't be
  provisioned by a git-driven tool. Reads from `infra/ansible/secrets.yml`
  (gitignored, `secrets.yml.example` is the template — same pattern as
  `terraform.tfvars`/`terraform.tfvars.example`) and `infra/ansible/argocd_deploy_key`
  (gitignored, generated with `ssh-keygen`, public half added as a GitHub read-only deploy key).
  Runs entirely `delegate_to: localhost` against the locally-fetched kubeconfig (never touches
  the remote VM's shell/process list), and self-skips (doesn't fail) each part whose input file
  is missing/still a placeholder. See `infra/ansible/README.md` ## GitOps-Secrets and
  `gitops/README.md` ## Privates Repo / ## DNS/ACME.
- The ArgoCD install task uses `kubectl apply --server-side --force-conflicts`, not plain
  `apply`: plain (client-side) apply stores the whole previous config in a
  `last-applied-configuration` annotation, and ArgoCD's `applicationsets.argoproj.io` CRD is big
  enough to blow past the Kubernetes API's hard 262144-byte annotation-size limit ("Too long"),
  failing the apply after most other objects already got created. Server-side apply doesn't write
  that annotation at all. Found on a real run against DHBWCloud VMs.
- Pinned versions in `inventory/group_vars/all.yml`: k3s `v1.37.0+k3s1`, ArgoCD `v3.4.5` (latest
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
# Fork personalization (only when deploying under a different DHBW account/repo). There is no
# script for this any more — scripts/personalize.py and deploy.env.example were deleted, since
# there is nothing left to spread around: edit gitops/env/cluster.yaml, plus the single literal
# repoURL in the git-files generator of gitops/applicationset.yaml. Secrets stay where they were
# (terraform.tfvars / secrets.yml). ArgoCD only ever reads the pushed remote state, so commit AND
# push before expecting anything to roll out.

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
ansible-playbook playbooks/bootstrap-gitops.yml   # re-run only the ArgoCD/ApplicationSet step
export KUBECONFIG=infra/ansible/kubeconfig
kubectl get nodes -o wide
kubectl -n argocd get pods

# Ansible static checks (no VMs needed)
cd infra/ansible
ansible-playbook playbooks/site.yml --syntax-check
ansible-lint

# catalog-api + frontend local dev stack (docker-compose, repo root — no Go/Node install needed
# locally, everything builds inside containers)
docker compose up -d --build
docker compose down -v

# catalog-api build/vet without a local Go toolchain
cd src/catalog-api
docker run --rm -v "$PWD":/src -w /src golang:1.25-alpine sh -c "go vet ./... && go build -o /tmp/catalog-api-bin ."

# helm lint/template without a local helm binary (any chart under charts/) - on Windows Git Bash,
# MSYS_NO_PATHCONV=1 is needed or the -v path gets mangled
MSYS_NO_PATHCONV=1 docker run --rm -v "$PWD/charts":/charts alpine/helm:3.16.3 dependency update /charts/<name>
# NOTE: every chart now needs the central values file, otherwise it aborts with a `required`
# message (that is deliberate — see the gitops layer above). With a local helm binary:
helm lint charts/<name> -f charts/<name>/values.yaml -f gitops/env/cluster.yaml
helm template <name> charts/<name> -f charts/<name>/values.yaml -f gitops/env/cluster.yaml
# charts/openbao's dependency-alias values ("openbao:") are easy to get wrong the first time
# (e.g. server.image.repository must NOT include the registry, unlike most other charts here —
# see Current implementation status) - always render and grep the actual image/env lines instead
# of trusting a clean `helm lint` alone:
helm template openbao charts/openbao -f charts/openbao/values.yaml -f gitops/env/cluster.yaml | grep -A2 "image:"

# OpenBao bootstrap check (after ArgoCD has synced openbao/openbao-postgres - the bootstrap Job
# runs automatically as a Helm post-install/post-upgrade hook, no separate command needed)
export KUBECONFIG=infra/ansible/kubeconfig
kubectl -n openbao get pods
kubectl -n openbao exec openbao-0 -- bao status
kubectl -n external-secrets get clustersecretstore openbao
kubectl -n ticketflow get externalsecret
```

`load-generator` is a plain k6 script (`src/load-generator/flash-sale.js`), not a Helm
chart/Kubernetes workload — run it directly with `k6 run`, see its README. `charts/notification-worker/`, `charts/reservation-worker/`
and `charts/akhq/` **are all** deployed via their own ApplicationSet elements (corrected here
2026-09-22 — this line previously and incorrectly claimed notification-worker had no element yet)
and all pass `helm lint`/`template` locally — see their bullets under Current
implementation status for what's still unverified beyond that (live cluster behavior,
placeholder values). `src/reservation-worker`: `go mod tidy && go vet ./... && go build ./... &&
go test ./...` all pass (verified locally 2026-09-14 with Go 1.27; `go.sum` was missing until
this run and is now committed), and `docker build src/reservation-worker` succeeds. Add real
commands here once these gaps close instead of leaving this section stale.

## Current implementation status (update this section as work lands)

- **Done:** `infra/terraform/` (tested by user), `infra/ansible/` (written, verified both
  statically and against real DHBWCloud VMs — k3s server/agent join and IPv6 node registration
  confirmed working; three real bugs found+fixed along the way, see Ansible specifics above), repo
  scaffold + all folder-level READMEs, root
  `README.md` skeleton with all required sections (many still marked `TODO` pending the pieces
  below).
- **Done, verified live on a real (rebuilt, dual-stack) cluster:** `gitops/applicationset.yaml`,
  `longhorn.yaml`, `cert-manager.yaml`, `cert-manager-issuer.yaml`, `external-dns.yaml`, and
  `argocd-ingress.yaml` — cluster addons ported from k3s-dhbw-cloud-role, deployed via ArgoCD/Helm
  instead of the Ansible roles on branch `ansible-bootstrap` (see Infra layering above), plus a
  public, TLS-passthrough ArgoCD ingress (see `gitops/README.md` ## ArgoCD-UI öffentlich
  erreichbar). **external-dns does not auto-publish a record for `IngressRoute`/
  `IngressRouteTCP`/`-UDP`** — unlike plain `Ingress`, its `traefik-proxy` source reads the
  target *only* from the `external-dns.alpha.kubernetes.io/target` annotation, never from the
  Traefik `Service`'s `LoadBalancer` status (confirmed against `source/traefik_proxy.go`
  upstream); `charts/argocd-ingress/templates/ingressroute-tcp.yaml` sets that annotation to a
  stable hostname (`values.yaml` → `externalDnsTarget`, `traefik.<zone>`) rather than raw VIPs.
  That hostname is published by the `traefik-config` ApplicationSet element + `charts/traefik-config/`, a
  `HelmChartConfig` (`helm.cattle.io/v1`) that merges an `external-dns.alpha.kubernetes.io/
  hostname` annotation into k3s' own bundled Traefik `HelmChart` (the documented way to add
  values to a k3s-managed chart without fighting its `helm-controller` for ownership) — for the
  Traefik `Service` itself, external-dns' `service` source *can* auto-derive the target from
  `LoadBalancer` status, so `traefik.<zone>` tracks VIP changes automatically and
  `argocd.<zone>` effectively CNAMEs to it. No manual step after a cluster rebuild. Every
  `Application` points at its own chart under `charts/`: wrapper charts
  (`Chart.yaml.dependencies` on the upstream chart) for `charts/longhorn/`, `charts/cert-manager/`,
  `charts/external-dns/`, fully-custom charts for `charts/cert-manager-config/` (the
  `ClusterIssuer`) and `charts/argocd-ingress/`. All use the real DHBW DNS zone
  (`cas420134-at-cas-dhbw-de.users.dhbw.site`) and `syncPolicy.automated`. The RFC2136 TSIG
  secret itself is deliberately **not** in the repo — `infra/ansible/roles/gitops_secrets`
  creates the K8s `Secret`s it needs from a gitignored local `infra/ansible/secrets.yml`
  (template: `secrets.yml.example`), run automatically at the end of `playbooks/site.yml` (see
  Ansible specifics above and `infra/ansible/README.md` ## GitOps-Secrets). The former
  `app-of-apps.yaml` had to exclude itself from its own directory source
  (`source.directory.exclude`) — without it, self-tracking + `prune: true` risked ArgoCD deleting
  its own root Application (and cascading through the `resources-finalizer` to every child app).
  That whole failure mode is gone with the ApplicationSet, which does not see itself as a managed
  resource; see `gitops/README.md` ## Struktur.
- **Done (2026-09-17), not yet live-verified:** central-config + ApplicationSet refactor. Replaces
  the previous `scripts/personalize.py` + `deploy.env.example` search-and-replace approach (both
  **deleted**) and the 19 `gitops/apps/*.yaml` files (also deleted) with `gitops/env/cluster.yaml`
  + `gitops/applicationset.yaml` — see the gitops layer above for the mechanics. What changed
  beyond that, and is worth knowing before touching any of it:
  - `charts/cert-manager-config`, `charts/argocd-ingress`, `charts/traefik-config`,
    `charts/ticketflow-app`, `charts/reservation-worker`, `charts/notification-worker` read their
    identity/image values from `.Values.ticketflow.*`. Each declares an empty `ticketflow: {}`
    in `values.yaml` purely so a render without the central file fails with a readable `required`
    message instead of a nil-pointer.
  - Two wrapper charts needed the value *inside* an upstream subchart's values, where Helm does
    not template. `charts/akhq` disables `akhq.ingress` and renders its own
    `templates/ingress.yaml`; `charts/external-dns` adds `templates/zone-configmap.yaml` and
    references it from the upstream values as `$(DNS_ZONE)`/`$(RFC2136_ZONE)`/`$(RFC2136_KEYNAME)`
    — Kubernetes expands `$(VAR)` in `args`, the same mechanism the chart already used for the
    TSIG secret. **No index-based `helm.parameters`** — those break on any reordering of
    `extraArgs`.
  - Namespaces themselves are centralized too: `ticketflow.namespaces.{app,kafka}` plus
    `ticketflow.kafka.clusterName` / `ticketflow.postgres.databaseName`. Every client derives its
    Kafka bootstrap address, Apicurio URL, Postgres host and Postgres Secret name from those
    instead of spelling them out — the Kafka bootstrap DNS alone used to be written out in four
    `values.yaml` in parallel. `charts/akhq` is the one exception and uses a short in-namespace
    name, because its value lives in the wrapped upstream chart's values where Helm does not
    template.
  - `infra/ansible/roles/gitops_secrets` and `infra/terraform/main.tf` read the same file
    (`include_vars` / `yamldecode`). `var.name_prefix` now defaults to `null` and falls back to
    `ticketflow.namePrefix` via `coalesce`.
  - **Migration on a running cluster is not a plain apply**: `app-of-apps` carries
    `resources-finalizer`, so deleting it normally cascades into every Application and its
    workloads. `kubectl -n argocd patch app app-of-apps --type=merge -p
    '{"metadata":{"finalizers":null}}'` → `kubectl -n argocd delete app app-of-apps
    --cascade=orphan` → `kubectl apply -f gitops/applicationset.yaml`, which then adopts the
    orphans by name. Documented in `gitops/README.md`.
  - Verified locally: all 19 charts `helm template`/`lint` clean against the central file,
    `tofu validate`, both Ansible playbooks `--syntax-check`, and an Ansible `include_vars` run
    proving the path resolves. **Not** verified: any of it against a live cluster (none
    available), and the ApplicationSet's own rendering by a real ArgoCD — the `templatePatch`
    output was checked by re-implementing its logic in Python, not by ArgoCD itself.
- **Committed but not yet re-verified live** (check `kubectl -n argocd get applications` before
  assuming any of this is synced clean — several commits landed after the last confirmed-live
  check): the Longhorn/cert-manager/external-dns wrapper-chart restructure, the ArgoCD/Longhorn/
  cert-manager/external-dns version bumps (latest stable as of this writing), and five new
  wrapper charts + Applications:
  - the `cnpg-operator` ApplicationSet element (`charts/cnpg-operator/`, CloudNativePG `v1.30.0`/chart
    `0.29.0`, cluster-wide, namespace `cnpg-system`)
  - `strimzi-operator.yaml` (`charts/strimzi-operator/`, Strimzi `1.1.0`,
    `watchAnyNamespace: true`, namespace `kafka`)
  - `kube-prometheus-stack.yaml` (`charts/kube-prometheus-stack/`, chart `87.19.0`, namespace
    `monitoring`; Grafana service ends up named `kube-prometheus-stack-grafana` — matches what
    root `README.md` ## Betrieb already assumed)
  - `loki.yaml` (`charts/loki/`, chart `7.1.0`, `SingleBinary` mode + bundled MinIO, namespace
    `monitoring`)
  - `tempo.yaml` (`charts/tempo/`, chart `1.24.4`, local storage backend, namespace `monitoring`)

  cnpg-operator/strimzi-operator/kube-prometheus-stack need `syncOptions: [ServerSideApply=true]`
  (see Infra layering above) — already set on all three; Loki/Tempo don't ship CRDs, don't need
  it. All eight workload charts (the five above plus Longhorn/cert-manager/external-dns) now set
  explicit `resources.requests`/`resources.limits` and enable the chart's own `networkPolicy`
  where one exists AND is safely scoped (cert-manager, kube-prometheus-stack, Tempo —
  external-dns/CNPG/Strimzi/Longhorn have no native NetworkPolicy field, deliberately not
  hand-built). **Loki's `networkPolicy.enabled` is deliberately `false`**, unlike the others —
  its chart unconditionally adds a `<release>-namespace-only` policy with an **empty
  `podSelector: {}`**, which applies to every pod in the shared `monitoring` namespace, not just
  Loki's own. Caught live: it cut Grafana's sidecar containers off from the Kubernetes API
  server (same namespace, different traffic target) → CrashLoopBackOff
  ("connection refused" on the API server's ClusterIP). No chart-level sub-toggle exists to
  disable just that one policy, so Loki's `networkPolicy` stays off entirely; see
  `charts/README.md` ## Resource-Limits, Security-Context, NetworkPolicy. **Before enabling
  `networkPolicy.enabled` on any future chart sharing this namespace, render it first and check
  the `podSelector` isn't empty/missing** — that's the actual failure signature to watch for.
  Most charts already ship hardened `securityContext` defaults (`runAsNonRoot`,
  `readOnlyRootFilesystem`, `capabilities.drop: [ALL]`) and were left alone; only Strimzi (`{}`
  by default) and Tempo (missing a couple of container-level fields) needed explicit overrides,
  and Longhorn has no security-context field at all (needs privileged host access for
  iSCSI/block devices — an unavoidable, documented exception, not an oversight). Same doc also
  covers a pod-vs-container-securityContext nesting bug caught and fixed while doing this (Tempo
  splits them across `tempo.tempo.securityContext` vs `tempo.securityContext` — don't assume one
  chart's values shape applies to another without checking the template first).
- **A chart-owned Secret must never share a name with a `KafkaUser` (found live, fixed):** the
  Strimzi User Operator names the SCRAM Secret it generates **exactly** after the `KafkaUser` CR,
  in that CR's namespace - which since the namespace split is `ticketflow`, the same namespace the
  app charts deploy into. `charts/notification-worker` created its own Secret at
  `<release>` = `notification-worker`, i.e. the same object: ArgoCD's `selfHeal` and the User
  Operator overwrote each other, the Deployment's `envFrom: secretRef` pulled whichever content
  won, and the `KAFKA_SASL_PASSWORD` `secretKeyRef` on key `password` broke whenever ArgoCD wrote
  last. Fixed by a `notification-worker.secretName` helper (`<release>-credentials`, overridable
  via `secret.name`), used by both `secret.yaml` and the Deployment's `envFrom`. **Before adding a
  Secret to any chart deploying into `ticketflow`, check it against `kafka.users` in
  `charts/kafka-cluster/values.yaml`** (`catalog-api`, `reservation-worker`, `notification-worker`,
  `akhq`). The other charts were checked and are clean.
- **Cross-namespace `KafkaUser` watching needs TWO settings, not one (found live, fixed):**
  `charts/kafka-cluster` sets `spec.entityOperator.userOperator.watchedNamespace` to the app
  namespace so the User Operator reconciles the `KafkaUser` CRs that live next to their consumers.
  On its own that makes the Cluster Operator **delete the Entity Operator deployment** again
  ("Entity Operator deployment deleted because Topic Operator and/or User Operator are configured
  with watchedNamespace set to a different namespace but the feature is disabled"). The gate is
  `STRIMZI_ENTITY_OPERATOR_WATCHED_NAMESPACE_ENABLED=true` on the **Cluster Operator** deployment;
  the upstream chart has no dedicated value for it, so `charts/strimzi-operator/values.yaml`
  injects it via `extraEnvs` (verified rendered into the Deployment, not just that the chart
  renders). RBAC needs nothing extra: with `watchAnyNamespace: true` the chart emits a
  `ClusterRoleBinding` for the entity-operator delegation. Treat the two as one unit.
- **Done, merged to `main`:** `charts/kafka-cluster` Kafka
  cluster — `Chart.yaml`/`values.yaml`/`values-dev.yaml`/`values-prod.yaml`/`values.schema.json`
  (schema currently only validates the `kafka.*` section; extensible for later sections) +
  `templates/messaging/`: `Kafka` CR in KRaft mode (no ZooKeeper, `strimzi.io/kraft: enabled`,
  `metadataVersion` set) with **separate `broker`/`controller` `KafkaNodePool`s** (not
  combined-role — restructured to match a real Strimzi setup the user provided as a reference
  from their workplace), each with a soft cross-host `podAntiAffinity` (from that reference) plus
  a hard `topologySpreadConstraint` on `topology.ticketflow/region` for P4 (only once that pool's
  `replicas > 1`, i.e. with `values-prod.yaml` — dev intentionally runs one broker + one
  controller); a `kafka-metrics` `ConfigMap` (JMX Prometheus Exporter rules, KRaft-specific rules
  enabled since KRaft is our only mode) wired in via `spec.kafka.metricsConfig`, plus
  `spec.kafkaExporter` for consumer-lag metrics (P7); two `KafkaTopic`s,
  `reservation-requested`/`reservation-result`, matching `RESERVATION_TOPIC`/`RESULT_TOPIC` from
  the service READMEs, each with `retention.ms`/`cleanup.policy` (prod also `min.insync.replicas:
  2`). Deliberately **not** copied from the reference: external listener/TLS/SCRAM auth (no
  external access needed), `authorization: type: simple` (no `KafkaUser`s exist yet — natural
  follow-up once `catalog-api`/`reservation-worker`/`notification-worker` exist),
  `inter.broker.protocol.version` (ZK-era field, meaningless under pure KRaft),
  `message.max.bytes: 200MB` (no justification for our small JSON events, left at the Kafka
  default). the `kafka-cluster` ApplicationSet element (`targetRevision: main`, namespace
  `ticketflow`, syncs `values-dev.yaml`). Strimzi chart version is pinned as `1.1.0`, which
  doesn't exist in reality (this project's timeline is deliberately forward-dated). First live
  sync attempt failed: the installed CRDs serve `kafka.strimzi.io/v1`, not the `v1beta2` this was
  originally written against — fixed (all other fields matched `kubectl explain` against the
  real CRD unchanged, confirmed via a server-side dry-run apply). AKHQ now exists (see
  the `akhq` ApplicationSet element below); a schema registry (also Carlos' scope) is still **not** part of
  this yet — separate follow-up, registry implementation undecided.
- **Done, verified locally via `docker-compose.yml` (repo root, local dev only — not part of the
  Terraform/Ansible/ArgoCD/Helm chain) against real Postgres + Kafka (KRaft, `apache/kafka:3.8.0`
  single-node) and a headless-Chromium click-through:** `src/catalog-api/` (Go 1.25 + `chi`,
  `pgx/v5` read-only DB access, `segmentio/kafka-go` producer, routes `GET /events`,
  `GET /events/{id}/seats`, `POST /events/{id}/reservations`, `GET /reservations/{id}`,
  `/healthz`/`/readyz`/`/metrics`, multi-stage Dockerfile → `distroless/static-debian12:nonroot`)
  and `src/frontend/` (plain HTML/CSS/JS, no build step, event picker + seat grid + booking form +
  status polling, `nginx:1.27-alpine` Dockerfile). The frontend renders **all** events from
  `GET /events` into a `<select>` (remembered in `localStorage`, falling back to `events[0]`), not
  just the first one. A hall is 40x25 = 1000 seats, so the seat grid uses one delegated click
  listener on `#hall` plus `data-seat-id` (never 1000 listeners) and builds into a
  `DocumentFragment`; seat/column sizes are `:root` CSS variables and `.hall-scroll` scrolls
  horizontally so the page itself never does. Verified in headless Chromium against a mock API at
  1280/900/500px and, via an iframe, at 390px - see `src/frontend/README.md` ## Darstellung von
  1000 Plaetzen. `local/postgres/init.sql` seeds three events in three
  venues, 1000 seats each (40 rows x 25), all `free` — identical SQL to
  `src/catalog-api/internal/db/migrate.go`, which is what actually seeds the cluster DB (the
  `docker-entrypoint-initdb.d` hook only exists in the plain `postgres` image, not in CNPG). catalog-api deliberately never writes to `reservations` —
  only the (not-yet-built) `reservation-worker` will; `GET /reservations/{id}` reports `pending`
  from an in-memory cache until that row exists, so the full `pending`→`confirmed`/`rejected`
  transition can't complete locally yet (confirmed as expected during testing, not a bug) until
  `reservation-worker` (see below) is actually deployed alongside it. The `ReservationRequested`
  Kafka payload (see `src/catalog-api/README.md` ## Kafka-Event-Schema) is now matched
  1:1 by `reservation-worker`'s `event.Requested`. `docker-compose.yml`
  includes a `kafka-init` one-shot job that pre-creates the topic — without it, the *first*
  request after a fresh stack start fails (`Unknown Topic Or Partition`) because Kafka's
  auto-topic-creation-on-first-produce is asynchronous to that same produce call; found live
  during E2E testing.
- **Done:** `src/notification-worker/` — first implemented service in the repo (Go), the second,
  independent Kafka consumer group off `RESULT_TOPIC`. `github.com/segmentio/kafka-go`
  (pure-Go, no cgo, needed for the distroless build). Config entirely via env vars
  (ConfigMap/Secret, see the package's own README table). Notification channel is pluggable:
  `LogNotifier` (default, demo-safe) or an optional `SMTPNotifier` gated by `SMTP_ENABLED`, with
  optional Kafka mTLS (`KAFKA_TLS_ENABLED`) structured the same way. Delivery semantics: the
  Kafka offset is committed **only on actual successful send**; a message that exhausts
  `NOTIFY_MAX_RETRIES` is published to an optional dead-letter topic (`DLT_TOPIC`) for
  visibility but stays uncommitted (redelivered on restart/rebalance) — a deliberate
  at-least-once trade-off, see `internal/kafkaconsumer` package doc. `/metrics`
  (`notifications_sent_total`, `notification_dispatch_errors_total`,
  `notification_dlt_published_total`), `/healthz`, `/readyz` (reflects Kafka reachability, not
  message arrival). Paired with its own **standalone** Helm chart `charts/notification-worker/`
  — deliberately *not* part of the umbrella chart below (a fully-custom, pattern-(b) chart per
  `charts/README.md`, first chart in the repo with `_helpers.tpl` since it renders 5 resources:
  Deployment/ConfigMap/Secret/PodDisruptionBudget/PodMonitor, plus an optional NetworkPolicy —
  deliberately no `Service`: nothing else consumes its HTTP endpoints (pure Kafka consumer,
  kubelet probes hit the pod directly), so a `PodMonitor` scrapes `/metrics` straight off the pod
  label selector instead of the usual `Service`+`ServiceMonitor` pair), `replicaCount: 3` +
  `PodDisruptionBudget` by default, and its own `values.schema.json` (the only per-service chart
  in the repo with one so far — everywhere else a schema is tied to the not-yet-started umbrella
  chart). No the `notification-worker` ApplicationSet element yet (out of scope for now — chart is
  lint/template-only, not deployed via ArgoCD). Go deps and both Dockerfile base images
  (`golang`, `distroless/static-debian12`) are pinned to latest-stable and digest-pinned (not
  floating tags) — re-check before rebuilding long after this was written.
- **Done, verified locally (2026-09-14, Go 1.27: `go mod tidy && go vet ./... && go build ./...
  && go test ./...` all pass, `go.sum` now committed):** `src/reservation-worker/` — the
  second Kafka consumer in the repo, the one that does the actual booking (Carlos' "Kafka
  Consumer" task, see root `README.md` ## ToDo). Consumes `RESERVATION_TOPIC`
  (`event.Requested`, exactly `catalog-api`'s already-published schema), claims the seat via
  **optimistic locking on `seats.status`** (`UPDATE ... WHERE status = 'free'`, no
  `SELECT ... FOR UPDATE`, per the service's own README), writes `reservations` and publishes
  `event.Result` to `RESULT_TOPIC` — exactly `notification-worker`'s already-defined consumer
  schema (`internal/event/event.go`), so no schema was renegotiated, just matched on both ends.
  Idempotent on `reservation_id` (redelivery after a crash between DB commit and offset commit
  re-derives the same result instead of double-claiming a seat — see `internal/booking` package
  doc). Builds its own DB schema at startup via embedded, versioned SQL migrations
  (`internal/migrate`, `schema_migrations` table, `CREATE TABLE IF NOT EXISTS` so it's safe
  against both an empty CNPG database and the already-seeded local `docker-compose` DB) instead
  of a separate migration tool — this is the "DB Schema aufbauen und über Applikation
  versionieren" part of the task. `/metrics`
  (`reservations_processed_total{result}`, `reservation_processing_duration_seconds{result}`,
  `reservation_processing_errors_total{stage}`), `/healthz`, `/readyz` (actively pings DB *and*
  Kafka, like `catalog-api`, unlike `notification-worker`'s simpler always-ready-once-connected
  check). Paired with its own standalone Helm chart `charts/reservation-worker/` (same
  Deployment/ConfigMap/Secret/PodDisruptionBudget/PodMonitor/NetworkPolicy/`_helpers.tpl`/
  `values.schema.json` pattern as `charts/notification-worker/`; `replicaCount: 1` is now just the
  Deployment's starting value — KEDA (`templates/keda-scaledobject.yaml`, added 2026-09-16, see
  the P6/KEDA bullet below) owns actual replica count from the first scale reconcile on, and
  `PodDisruptionBudget` is enabled by default now that scaling is real), deployed via
  the `reservation-worker` ApplicationSet element.

  **Postgres wiring (resolved 2026-09-15, credentials hardcoded 2026-09-16):** originally blocked
  by the same cross-namespace-secret gap as everywhere else (`charts/postgres-cluster` only
  creates a `username`/`password` `Secret` in namespace `application`, this chart deployed into
  `ticketflow`). Resolved per Niklas' team-wide decision (see the `catalog-api`/`ticketflow-app`
  bullet below): the chart creates its **own** `Secret` (`templates/secret.yaml`,
  `databaseSecret.{create,username,password}`) instead of referencing an externally copied one.
  `username`/`password` were `CHANGE_ME` placeholders until 2026-09-16, then hardcoded to
  `postgres`/`ticketflow` on explicit user request (same lab-credentials reasoning as the AKHQ
  passwords — Uni project, not live, simple/consistent over CHANGE_ME-then-forget) — must match
  `charts/postgres-cluster`'s `database.owner`/`.password`, hardcoded there too now. The app
  builds its own connection string from `DB_HOST`/`DB_PORT`/`DB_NAME`/
  `DB_SSLMODE` (ConfigMap, `values.yaml` → `database.*`, real values) plus `DB_USER`/
  `DB_PASSWORD` (explicit `secretKeyRef`s against that Secret's `username`/`password` keys, not
  `envFrom` — the key names don't match the env var names). `helm lint`/`template` clean with no
  overrides needed.
  Also added a `reservation-worker` service + `reservation-result` topic pre-creation to the
  root `docker-compose.yml` for local testing (previously only had `reservation-requested`).
- **Done for `charts/reservation-worker`'s half (`helm lint`/`template` clean), `charts/keda`
  itself not yet `helm dependency update`-able locally (2026-09-16):** P6 — KEDA scales
  `reservation-worker` on Kafka consumer-group lag instead of CPU (root `README.md` ## P6).
  `charts/keda` (new, wrapper around the upstream `kedacore/charts` chart pinned `2.20.2`) +
  the `keda` ApplicationSet element (namespace `keda`, cluster-wide `watchNamespace` default kept,
  `syncOptions: [ServerSideApply=true]` — its `scaledjobs` CRD alone is >650KB, same class of
  problem as CNPG/Strimzi/kube-prometheus-stack) is the operator only, same split as
  `charts/cnpg-operator` vs. `charts/postgres-cluster`: the actual `ScaledObject`/
  `TriggerAuthentication` live with the workload they target, in
  `charts/reservation-worker/templates/keda-scaledobject.yaml`, not in `charts/keda`. Written
  against the upstream chart's real `values.yaml` structure and the actual `keda.sh` Kafka-scaler
  docs (fetched live, not guessed) — `crds.install`/`podSecurityContext`/`securityContext` already
  sensible chart defaults, kept; `resources` set explicitly per component (operator/metricServer/
  webhooks, scaled down for the lab cluster, same sizing convention as `charts/cnpg-operator`/
  `charts/strimzi-operator`); the chart's native `networkPolicy` field is enabled (`flavor:
  kubernetes`, no Cilium in this cluster) — **not yet rendered/verified** that its per-component
  policies are safely scoped (the "Loki-bug" class of empty-podSelector risk, see `charts/README.md`
  ## Resource-Limits...), since `charts/keda`'s dependency can't be resolved locally right now (see
  below); `prometheus.operator.serviceMonitor.enabled: true` wired to `kube-prometheus-stack`
  (P7). **`charts/keda` itself could not be `helm dependency update`d/lint/template-verified in
  this session** — same corporate TLS-interception proxy that blocked the `docker compose`
  Go-module downloads earlier also breaks any HTTPS fetch from inside a container on this
  machine (confirmed: even a bare `curl https://kedacore.github.io/...` from a container fails
  with "self-signed certificate in certificate chain") — this repo already has precedent for
  committing a wrapper chart this way (`charts/cnpg-operator`/`charts/strimzi-operator` were never
  locally dependency-resolved either, per their own status entries above), so this isn't a new
  class of gap, but it does mean `charts/keda`'s rendered output is unverified beyond "the values
  key names match the real upstream `values.yaml` structure I fetched" — check with a real
  `helm template` (or just watch the live ArgoCD sync) before trusting it blind.

  `charts/reservation-worker`'s half **is** fully `helm lint`/`template`-verified locally (no
  external dependency to resolve there, pure custom templates): a small chart-owned Secret
  (`<fullname>-keda-auth`) holds just the SASL **username** (`reservation-worker`, not sensitive)
  because KEDA's Kafka scaler only accepts `username`/`password` via `TriggerAuthentication.
  secretTargetRef` (confirmed against the live `keda.sh/docs/latest/scalers/apache-kafka` docs,
  never as a literal metadata value) and the Strimzi-generated Secret
  (`config.kafkaSasl.secretName`) has no separate `username` key to reuse — the **password**
  comes straight from that existing Strimzi Secret, no copy. `config.keda.{minReplicaCount,
  maxReplicaCount,lagThreshold,pollingInterval,cooldownPeriod}` are new values, deliberately NOT
  defaulted to KEDA's own generic defaults: `maxReplicaCount: 3` is pinned to
  `reservation-requested`'s actual partition count on the profile that's actually synced right
  now (the `kafka-cluster` ApplicationSet element runs `values-dev.yaml`, 3 partitions — a consumer group
  can't usefully run more consumers than partitions) and needs bumping to 8 alongside any future
  switch to `values-prod.yaml`; `lagThreshold: "10"` is deliberately low against the 1000 seats per
  venue (three events, see `local/postgres/init.sql` / `src/catalog-api/internal/db/migrate.go`),
  so a flash-sale burst reliably scales out to `maxReplicaCount` without the threshold being out
  of reach for smaller load tests; `pollingInterval: 15`/`cooldownPeriod: 60` are shorter than KEDA's defaults
  (30s/300s) so a screencast shows both scale-up and scale-down without a multi-minute wait.
  `replicaCount: 1` in this chart is now purely the Deployment's starting value, not an ongoing
  source of truth — the `reservation-worker` ApplicationSet element gained an `ignoreDifferences` entry on
  `spec.replicas` for the Deployment (the standard ArgoCD + HPA/KEDA pattern), otherwise
  `selfHeal: true` would stomp every KEDA scale event back to `replicaCount: 1` on the next sync
  tick. `podDisruptionBudget.enabled` flipped from `false` to `true` (`minAvailable: 1`) now that
  the replica count is no longer statically 1.
- **Done, `helm lint`/`template` clean for all three (2026-09-16):** P4 scheduling/placement for
  `charts/reservation-worker`/`charts/akhq`/`charts/apicurio` — previously only
  `charts/ticketflow-app` and the Kafka/Postgres node pools had a region-spread preference; these
  three didn't. `reservation-worker`/`apicurio` (own templates, `_helpers.tpl`-based) got the same
  `topologySpreadConstraints` + `whenUnsatisfiable: ScheduleAnyway` block as `charts/ticketflow-app`,
  gated by a new `topologySpreadConstraint.{enabled,maxSkew}` value.
  **`charts/akhq` needed a different mechanism, found only by checking the vendored upstream chart's
  actual `templates/deployment.yaml`:** its `values.yaml` doesn't have a `topologySpreadConstraints`
  field at all — only `affinity`/`nodeSelector`/`tolerations` are templated (raw `toYaml`
  passthrough). A first attempt setting `topologySpreadConstraints:` there rendered into nothing,
  silently — caught only by actually grepping the rendered output for the block and finding it
  missing, not from `helm lint`, which stayed green either way. Fixed with an equivalent
  `affinity.podAntiAffinity.preferredDuringSchedulingIgnoredDuringExecution` term instead
  (`topologyKey: topology.ticketflow/region`), with the `labelSelector` hardcoded to AKHQ's actual
  rendered pod labels (`app.kubernetes.io/name`/`instance: akhq`) since `values.yaml` isn't
  templated and can't call a Helm helper the way the two custom charts do. **Lesson for any future
  wrapper-chart values.yaml edit that isn't covered by this repo's own `values.schema.json`:
  `helm lint`/`template` only prove a chart renders, never that a specific value you set actually
  reaches a template** — grep the rendered manifest for the field you expect, don't just check the
  exit code. All three effectively no-ops at `replicaCount: 1` (their current default everywhere),
  same as `charts/ticketflow-app`'s own dev profile — becomes real once any of them scales beyond 1
  (`reservation-worker` via KEDA, see above).
- **Done, `helm lint`/`template` clean, not yet live-verified:** `charts/akhq/` (wrapper around the upstream `akhq` chart,
  `https://akhq.io/`, pinned `0.28.0`) + the `akhq` ApplicationSet element — Kafka web UI, Carlos' scope.
  Deployed into namespace `kafka` (grouped with the Strimzi operator, not `ticketflow` — AKHQ is
  Kafka-platform tooling, not part of the TicketFlow app itself; cross-namespace access to the
  bootstrap service in `ticketflow` is fine, only `Secret`s are namespace-scoped, not `Service`s).
  `secrets.akhq.connections.ticketflow-kafka.properties.bootstrap.servers` points at the real
  Strimzi bootstrap service. Public via a plain `Ingress` (`akhq.<zone>`) + `cert-manager.io/
  cluster-issuer` annotation — deliberately simpler than the ArgoCD ingress: AKHQ speaks plain
  HTTP, no TLS-passthrough `IngressRouteTCP` needed, so external-dns' already-enabled `ingress`
  source (`charts/external-dns/values.yaml`) picks up the host automatically once Traefik
  assigns the Ingress a `LoadBalancer` address — no `traefik-config`-style workaround required.
  Secured with basic-auth (`micronaut.security.enabled` + `akhq.security`), RBAC roles adapted
  from AKHQ's own `application.example.yml` but restricted to the `*-read` roles only (no
  `*-admin` roles with CREATE/UPDATE/DELETE), matching root `README.md` ## Bonus (B2)'s
  explicit "AKHQ (schreibgeschützt konfiguriert)" claim. `default-group: no-roles` (not the
  upstream example's `admin`) so anonymous visitors get no access at all. Also authenticates to
  Kafka itself via SASL/SCRAM-SHA-512 (see B2/`charts/kafka-cluster` bullet below) — `extraEnv`
  injects a `KAFKA_SASL_JAAS_CONFIG` env var from a chart-owned `templates/kafka-sasl-secret.yaml`
  Secret (own resource, not passed through to the upstream `akhq:` dependency block), referenced
  as `${KAFKA_SASL_JAAS_CONFIG}` inside `secrets.akhq.connections.ticketflow-kafka.properties`
  (AKHQ/Micronaut resolves `${VAR}` placeholders from the environment at runtime, so that
  specific field never carries a literal password).
  **Both the basic-auth password and the Kafka SASL password described in the rest of this
  paragraph as hardcoded lab values are stale as of 2026-09-23** — see the OpenBao bullet's
  AKHQ-unification paragraph further below: the basic-auth hash is now
  `"${AKHQ_BASIC_AUTH_PASSWORD_HASH}"` sourced from OpenBao, `kafkaSasl.password` no longer exists
  in `values.yaml` at all (Strimzi generates AKHQ's Kafka password now, same as the other three
  KafkaUsers), and `kafka-sasl-secret.yaml` no longer reconstructs the JAAS string itself. `helm
  dependency update`/`lint`/`template` all pass (re-verified 2026-09-23 after this change) and the
  rendered Secret/ConfigMap/Deployment/Ingress all look correct — but this only proves the chart
  renders, not that AKHQ actually enforces the RBAC config or connects successfully via SASL as
  intended; both are still unverified against a running instance — check login + role behavior +
  the Kafka connection in the AKHQ UI after the first real sync.
- **Done, `helm lint`/`template` clean for all five affected charts, `go vet`/`build`/`test` clean
  for all three Go services, not yet live-verified (2026-09-16):** B2 (root `README.md` ## Bonus)
  — per-service `KafkaUser` CRs (SCRAM-SHA-512 + narrowest-possible `type: simple` ACLs) replacing
  the previously anonymous/unauthenticated Kafka access.
  `charts/kafka-cluster/templates/messaging/kafka-cluster.yaml`: the `plain` listener (name kept
  unchanged — it only affects internal Strimzi bookkeeping, not the stable
  `<cluster>-kafka-bootstrap` DNS every consumer already references) now sets
  `authentication.type: scram-sha-512` (still `tls: false`, i.e. SASL_PLAINTEXT, not SASL_SSL —
  transport stays unencrypted per the existing P1-P7 decision, only the login is now
  authenticated), and `spec.kafka.authorization: {type: simple}` is on (default-deny: a client
  with no matching ACL is rejected outright). New `templates/messaging/kafka-users.yaml`
  (generated from `values.yaml` `kafka.users`, four entries: `catalog-api`, `reservation-worker`,
  `notification-worker`, `akhq`). **Historical note, superseded 2026-09-23 (see the OpenBao
  bullet's AKHQ-unification paragraph further below) — kept here for context, not current
  behavior:** at the time this was written, three of the four users
  (`catalog-api`/`reservation-worker`/`notification-worker`, same namespace `ticketflow` as this
  Kafka cluster) let the Strimzi User Operator auto-generate their SCRAM password and were
  referenced directly via `secretKeyRef` from their own charts (no chart-owned secret needed,
  unlike the Postgres case) — `akhq` ran in namespace `kafka` instead, so Kubernetes'
  same-namespace-only `secretKeyRef` rule meant it couldn't read an auto-generated Secret from
  `ticketflow`; instead its `KafkaUser` set `spec.authentication.password.valueFrom.secretKeyRef`
  to a Secret this chart created itself (`kafka-akhq-user-secret.yaml`, deleted 2026-09-23), and
  `charts/akhq/values.yaml`/`templates/kafka-sasl-secret.yaml` supplied the matching value on the
  other side (both hardcoded to the lab password `ticketflow` at the time, same
  shape as the Postgres `databaseSecret` values elsewhere in this repo, also hardcoded rather than
  `CHANGE_ME` as of 2026-09-16 on explicit user request: Uni lab project, not a live system,
  weak-but-simple beats CHANGE_ME-then-forget). AKHQ now lets Strimzi auto-generate its password
  too, same as the other three, mirrored to OpenBao via `PushSecret` instead — see the
  `charts/akhq` bullet above for how AKHQ
  itself consumes it.
  `catalog-api`/`reservation-worker`/`notification-worker` charts each gained a
  `config.kafkaSasl`/`catalogApi.kafkaSasl` block (`enabled`/`username`/`secretName`) wired into
  their ConfigMap (`KAFKA_SASL_ENABLED`/`KAFKA_SASL_USERNAME`) and Deployment
  (`KAFKA_SASL_PASSWORD` via `secretKeyRef` against the Strimzi-generated Secret); all three
  `values.schema.json` updated to match. `notification-worker`'s long-standing placeholder
  defaults (`config.resultTopic: "reservation-results"`, `config.kafkaBrokers: ""`) were also
  fixed to the real topic name (`reservation-result`, singular — was a typo/never-updated
  placeholder) and the real bootstrap-service DNS while touching this file, even though that
  chart still has no the `notification-worker` ApplicationSet element Application (unrelated, still not
  started, see below). Go side: all three services gained SASL/SCRAM-SHA-512 support via
  `github.com/segmentio/kafka-go/sasl/scram` (already a transitive part of the already-vendored
  `segmentio/kafka-go` dependency, `go mod tidy` pulled in `xdg-go/{scram,pbkdf2,stringprep}` as
  new indirect deps in all three `go.sum` — no new direct dependency chosen) — a
  `KAFKA_SASL_ENABLED`/`_USERNAME`/`_PASSWORD` env-var triple, optional and default-`false` (same
  shape as `notification-worker`'s pre-existing `KAFKA_TLS_ENABLED` mTLS scaffold, but
  independent/orthogonal to it — our listener is SASL_PLAINTEXT, not SASL_SSL, so both could in
  principle be combined later but aren't here). `reservation-worker`/`notification-worker` each
  got a small `internal/kafkasasl` package (`Build(cfg) (sasl.Mechanism, error)`, nil when
  disabled) mirroring `notification-worker`'s existing `internal/kafkatls` package shape;
  `catalog-api` has no `internal/config` package at all (reads env directly in `main.go`), so its
  equivalent is an inline `buildKafkaSASL()` helper in `main.go` instead of a new package, for
  consistency with that file's existing flat style. Config validation (fail-fast at startup)
  requires username+password when `KAFKA_SASL_ENABLED=true`, covered by new unit tests in both
  `reservation-worker` and `notification-worker`'s `internal/config` packages. Local
  `docker-compose.yml` is deliberately untouched — it has no Strimzi listener/ACLs, so
  `KAFKA_SASL_ENABLED` stays unset (defaults to `false`) there, same plaintext local dev path as
  before; **not yet re-verified end-to-end via `docker compose up --build`** after this change —
  attempted once but blocked by this machine's corporate TLS-interception proxy rejecting `go mod
  download` inside the build containers (`x509: certificate signed by unknown authority` against
  `proxy.golang.org`), an environment limitation unrelated to this change (the same three services
  build/vet/test cleanly with the host's native Go 1.27 toolchain, outside Docker). Root
  `README.md` ## Bonus (B2) paragraph updated from "was getan werden soll" (planned) to "was
  umgesetzt wurde" (done), topic names corrected to the real singular `reservation-requested`/
  `reservation-result` (previous text had a plural-typo mismatch,
  `reservation-requests`/`reservation-results`, against the actual `KafkaTopic` names), namespace
  in the "Nachweis im Betrieb" `kubectl` example corrected from `-n kafka` to `-n ticketflow`
  (`KafkaUser`s live with the Kafka cluster, not with AKHQ), and the four-user/cross-namespace
  password mechanics added. B2 was already documented under root `README.md` ## Bonus (not `##
  Entscheidungen`), so no change was needed to this file's "Chosen elective topics" line at the
  top (that names only the two required W-topics, W1/W2; B1/B2 are separate optional bonus
  topics) — the previous flag about that line being out of sync was a misreading, not an actual
  gap. **Known open risk, unverified against a live cluster (documented in a comment in
  `kafka-cluster.yaml` itself):** Strimzi is *supposed* to automatically grant its own
  Cluster/Entity Operator (Topic/User Operator) super-user rights once
  `authorization: type: simple` is enabled, via an internal mTLS identity on a listener outside
  user control — if that assumption is wrong, `KafkaTopic`/`KafkaUser` reconciliation could break
  the moment this syncs for real; check the Strimzi operator logs first if `kubectl get
  kafkatopic`/`kafkauser` show errors after this deploys, and add an explicit
  `spec.kafka.authorization.superUsers` entry for the operator's principal CN as the fix if so.
- **Done:** `.github/workflows/build-images.yml` now actually builds and pushes (previously just
  a `docker build`-only TODO stub — all four matrix Dockerfiles already existed, only the push
  target was missing). Pushes to **ghcr.io** (GitHub Container Registry) as
  `ghcr.io/<owner-lowercase>/ticketflow-<service>:<git-short-sha>`, auth via the built-in
  `GITHUB_TOKEN` (`packages: write` permission, no extra secret) — chosen over a self-hosted
  in-cluster registry: ghcr.io is explicitly covered by "container registries/package sources are
  permitted external dependencies" (see What this repo is above), needs no public ingress/auth of
  its own for GitHub's external runners to push to, and decouples image lifecycle from cluster
  rebuilds (a self-hosted registry's images would be lost on rebuild same as the cluster). Owner
  is lowercased via `${GITHUB_REPOSITORY_OWNER,,}` (ghcr.io requires lowercase image names,
  the repo owner `Engelhan` is mixed-case). **Deliberately public, not private** — pure build
  artifacts, no reason to gate them, and public avoids needing a cluster-side `imagePullSecret`
  entirely. **Manual one-time step still needed**, not automatable via `GITHUB_TOKEN`: after the
  first successful run, set each of the four packages' visibility to "Public" in GitHub's package
  settings (defaults to the repo's private visibility otherwise) — until then pods get
  `ImagePullBackOff` even with a correct tag. `charts/reservation-worker/values.yaml` and
  `charts/notification-worker/values.yaml` `image.repository` no longer hold a registry path at
  all — every chart derives its image from `ticketflow.imageRegistry`/`.imageTag` in
  `gitops/env/cluster.yaml`, so a fork under a different GitHub owner changes one line. The
  workflow also gained `workflow_dispatch` (a fresh fork previously had no way to build images
  without artificially touching a file under `src/`) and a `bump-image-tag` job that writes the
  new short SHA back into that file — a single job after the matrix, not a step inside it, so the
  four parallel builds cannot race each other on push. No trigger loop: the workflow only fires on
  `src/**`, the commit only touches `gitops/`.
- **`reservation-worker` Application status (updated 2026-09-15, now an ApplicationSet element):** `automated`/`selfHeal`,
  namespace `ticketflow`, no `helm.values` override — no `values-dev.yaml`/`values-prod.yaml`
  split for this chart either, unlike `charts/kafka-cluster`/`charts/kafka-cluster`; all values come
  from `charts/reservation-worker/values.yaml` defaults directly. `image.repository`/`image.tag`/
  `config.kafkaBrokers`/`database.*` are all real values now (ghcr.io path + a real,
  ghcr.io-verified git-short-sha tag, Strimzi bootstrap service, real CNPG host/port/name/
  sslMode — see the Postgres-wiring paragraph above); `databaseSecret.{username,password}` are
  now hardcoded lab credentials (`postgres`/`ticketflow`, see the Postgres-wiring paragraph
  above), consistent with `notification-worker`/`catalog-api`'s same pattern. `helm lint`/
  `template` clean with zero overrides.

  **Real incident, 2026-09-15 (root-caused, now fixed):** an earlier commit removed the
  then-still-needed inline `helm.values` block from the Application without moving those values
  into the chart's own `values.yaml` first — since `values.schema.json` required
  `config.kafkaBrokers`/`secret.databaseUrl` non-empty (and forbade `image.tag: latest`), `helm
  template` failed schema validation entirely, so ArgoCD couldn't generate any manifest at all
  (sync status `Unknown`, `ComparisonError` in `status.conditions`) — the ArgoCD UI's generic
  fallback for that state is the misleading "revision main must be resolved" message, which is
  *not* an actual git/repo-resolution problem. Worth remembering as a class of failure: removing
  an Application's inline value override is only safe once the chart's own defaults already cover
  everything the schema requires — check `helm lint` with zero `--set` flags before removing one.
- **Done, `helm lint`/`template` clean, full local end-to-end verified via `docker compose`
  against a real Apicurio instance — not yet live on a cluster (no cluster available):** Apicurio
  Registry integration (full wire-format scope, decided 2026-09-15 — see memory
  `project-schema-registry-next` for the decision history; this deliberately touches
  `catalog-api`/Philipp's and `notification-worker`/Niklas's code, not just Carlos' own scope,
  flag to the team before merging branch `apicurio-schema-registry`). `charts/apicurio/` is a
  **fully custom chart** (Deployment/Service/ConfigMap), not the originally-planned wrapper
  around `eshepelyuk/apicurio-registry-helm` — that OCI package returns 403 Forbidden on every
  pull (verified via Docker and directly against ghcr.io's token endpoint; ghcr.io itself works
  fine for other packages), apparently no longer public since its archival; see
  `charts/README.md` ## apicurio for the full story including a second, independent finding that
  the chart was built for Apicurio 2.x's per-storage-image/env-var scheme, not 3.x's unified
  image + `APICURIO_STORAGE_KIND` env var. Storage is `kafkasql` (a Kafka topic + in-memory H2,
  no extra Postgres/cross-namespace-secret dependency). The two canonical JSON Schemas live at
  `charts/kafka-cluster/files/schemas/*.schema.json` and are registered idempotently by a new
  `post-install,post-upgrade` Helm hook Job in `charts/kafka-cluster/templates/schema-registry/`
  (`ifExists=FIND_OR_CREATE_VERSION&canonical=true`) — verified live: registered cleanly against
  a real registry, and confirmed the exact `globalId` response field name/casing that was only
  assumed beforehand. Each of the three Go services carries its own `internal/schemareg/` package
  (hand-rolled REST client + `github.com/santhosh-tekuri/jsonschema/v6` local validation,
  duplicated per service like `internal/event` already is, not a shared module) that wraps
  outgoing / unwraps incoming Kafka message payloads with a Confluent-compatible header (`0x0`
  magic byte + 4-byte big-endian `globalId`) and validates against a schema embedded in the
  binary via `go:embed` — no per-message registry round-trip, only `Resolve`/`Ping` (wired into
  each service's existing `/readyz`) hit the registry. A schema-invalid or malformed-frame message
  reuses each service's pre-existing poison-pill/DLT handling, no new failure path. Full local
  `docker compose up --build` run confirmed the entire flow end-to-end: a real reservation went
  `pending` → `confirmed` through all three services with schema validation/framing active, zero
  errors. `docker-compose.yml` also gained the previously-missing `notification-worker` service
  (had no entry at all before this) and an `apicurio`/`apicurio-init` pair mirroring the cluster
  setup. Known unverified-live gaps: `charts/apicurio`'s rendered Service DNS
  (`apicurio.kafka.svc:8080`) assumes the ArgoCD Application release name equals the Application
  name, same assumption pattern as every other addon chart here — not checked against a real
  cluster sync yet.
- **Done:** `charts/ticketflow-app` — standalone Helm chart for `catalog-api`+`frontend`
  (Deployment/Service/ConfigMap/ServiceMonitor/PodDisruptionBudget for catalog-api, same plus
  Ingress for frontend), deployed via the `ticketflow-app` ApplicationSet element (`targetRevision: main`,
  namespace `ticketflow`, shared with `charts/kafka-cluster`'s Kafka so catalog-api reaches the
  bootstrap service by short in-namespace DNS). Team decision: catalog-api/frontend must NOT be
  deployed in the same Application as Kafka — this chart was split out of `charts/kafka-cluster`
  (which originally held both) for exactly that reason; `lint`/`template`/kubeconform all pass
  for both charts post-split.
- **Done:** `src/load-generator/flash-sale.js` (k6) — ramping-VUs flash-sale scenario, `setup()`
  loads the free-seat list once (load should hit `POST /events/{id}/reservations`, not repeated
  seat lookups), a fixed `HOT_SEAT_RATIO` (default 20%) of iterations deliberately targets the
  same seat to produce real `409`/`ReservationRejected` conflicts instead of relying on random
  collisions. `202`/`409` both count as success (`options.thresholds`) — a rejected seat is a
  correct flash-sale outcome, not a system error. Verified locally against docker-compose (30
  VUs/15s: 878 requests, 694×`202`, 183×`409`, 0 unexpected errors); `reservation-worker` visibly
  lagged behind afterwards processing the Kafka backlog (single local instance, no KEDA) — this
  is exactly the consumer-lag signal KEDA reacts to in the real cluster (`charts/keda`, P6).
- **Not started:** a dedicated ArgoCD `AppProject` (everything still runs on `project: default`),
  `observability/dashboards/` + `observability/alerts/` content, and B1's project-wide
  deny-by-default `NetworkPolicy` set (see root `README.md` ## Bonus B1 — the NetworkPolicy half
  is still "was getan werden soll"; the secret-vault half is done, see the OpenBao/External
  Secrets Operator bullet below; three charts already have their own scoped `NetworkPolicy`
  active, see `charts/README.md` ## Resource-Limits...). The cross-namespace `DATABASE_URL`
  blocker mentioned in older versions of this line is resolved (see the Postgres-wiring paragraph
  above) — `reservation-worker`'s chart is deploy-ready.
- **Done, `helm lint`/`template`/`helm dependency update` all verified locally (2026-09-22), not
  yet live-verified:** OpenBao (HA secret vault, PostgreSQL storage backend) + External Secrets
  Operator, replacing the SOPS plan under root `README.md` ## Bonus B1 (see there for the full
  Wahl/Alternative/Begründung). Four new charts: `charts/openbao` (wrapper around the official
  `openbao/openbao` chart 0.29.5/`v2.6.2`, `server.ha.enabled: true` + `replicas: 3` +
  `raft.enabled: false`, `storage "postgresql" { ha_enabled = "true" }` in `server.ha.config`,
  `BAO_PG_CONNECTION_URL` injected via `extraSecretEnvironmentVars` from OpenBao's own CNPG
  cluster's auto-generated `<clusterName>-app` secret — **not** a chart-owned secret, see below —
  `topologySpreadConstraints` on `topology.ticketflow/region` for P4, an `auto-unseal` sidecar
  container polling `bao status` and unsealing from a mounted `openbao-unseal-keys` Secret), the
  `openbao` ApplicationSet element, `charts/openbao-postgres` (a second, separate CNPG `Cluster`
  CR chart, copy of `charts/postgres-cluster`'s structure incl. its normally-dormant
  backup/import blocks, but **deliberately omits `bootstrap.initdb.secret`** so CNPG generates
  and owns the credential itself — OpenBao's own storage-backend password can't come from OpenBao,
  same chicken-and-egg reasoning as why `charts/postgres-cluster`'s own credential stays
  chart-hardcoded rather than vault-sourced), the `openbao-postgres` ApplicationSet element,
  `charts/external-secrets-operator` (wrapper, chart 2.11.0/`v2.11.0`, `installCRDs: true`,
  `serverSideApply: true` — see the CRD-size note under Infra layering above), the
  `external-secrets-operator` ApplicationSet element, and `charts/openbao-secretstore` (a single
  `ClusterSecretStore` CR against `http://openbao-active.<namespaces.openbao>.svc:8200`,
  Kubernetes-auth), the `openbao-secretstore` ApplicationSet element. New `gitops/env/cluster.yaml`
  keys: `namespaces.openbao`, `namespaces.externalSecrets`, `openbao.databaseName`. No new Ansible
  role — bootstrap (init/unseal/KV-v2-engine/Kubernetes-auth-method/policy/seeding) runs entirely
  as a `post-install,post-upgrade` Helm-hook `Job` in `charts/openbao/templates/bootstrap-job.yaml`
  (same hook pattern as the existing Apicurio schema-registration job in
  `charts/kafka-cluster/templates/schema-registry/register-job.yaml`), reaching every OpenBao pod
  directly over the upstream chart's headless Service (`openbao-internal`, confirmed against the
  real rendered manifest — pod DNS `openbao-<N>.openbao-internal.<ns>.svc:8200`). **The pod list
  is generated from `.Values.openbao.server.ha.replicas` at Helm-render time** (`{{ range $i :=
  until $replicas }}`), not hardcoded to 3 — a replica-count change is itself a Helm
  upgrade/ArgoCD sync, which re-fires the `post-upgrade` hook with the newly-rendered pod list;
  no runtime Kubernetes-API discovery needed (a manual `kubectl scale` bypassing GitOps would get
  reverted by `selfHeal` anyway, since this Application carries no `ignoreDifferences` on
  `spec.replicas` the way `reservation-worker`/KEDA does). Verified by rendering with
  `--set openbao.server.ha.replicas=5` and confirming the pod list follows.
  Talks to the Kubernetes API directly via the mounted ServiceAccount token (own
  `bootstrap-rbac.yaml`: a `ServiceAccount`/`Role`/`RoleBinding` scoped to
  `get`/`create`/`update`/`patch` on `Secret`s in the `openbao` namespace only, no `pods/exec`).
  The job is idempotent by construction — it always re-reads any existing `openbao-unseal-keys`
  Secret first, so a fresh bootstrap and a later ArgoCD re-sync run the identical code path — and
  it randomly generates the purely-internal service-to-service credentials that have no operator
  to generate them (`secret/akhq/basic-auth`) itself, only if the KV path doesn't already exist
  (as of 2026-09-23, `secret/postgres/app-user`/`secret/kafka/*` are no longer generated here —
  see the PushSecret extension further below); `notification-worker`'s SMTP/Kafka-mTLS paths are seeded
  with empty placeholders since that feature stays opt-in (see
  `charts/notification-worker/README.md` ## OpenBao for the manual `bao kv put` to activate it
  later). Deliberate lab simplification: `bao operator init -key-shares=1 -key-threshold=1`
  (Shamir 1-of-1, not 3-of-5) — no offline backup of the root token/unseal key exists outside the
  cluster by design (a GitOps hook Job can't write to an operator's local machine the way an
  Ansible role could); the recommended mitigation is a one-time manual `kubectl -n openbao get
  secret openbao-unseal-keys -o yaml` backup after the first sync, documented in the README, not
  automated.

  Chart-owned Secrets migrated to `ExternalSecret`s pulling from this vault (target Secret
  name/keys stay byte-identical to before, zero Deployment changes):
  `charts/postgres-cluster/templates/postgres-user-secret.yaml`,
  `charts/akhq/templates/kafka-sasl-secret.yaml` (as of 2026-09-23 a straight 1:1 key copy of
  `sasl.jaas.config` — no template reconstruction, see below),
  `charts/akhq/templates/basic-auth-secret.yaml` — AKHQ's basic-auth password hash used to be a
  literal in `values.yaml`, now `"${AKHQ_BASIC_AUTH_PASSWORD_HASH}"`, the same Micronaut
  env-substitution trick already used for `${KAFKA_SASL_JAAS_CONFIG}`; **unverified at
  implementation time** whether `${VAR}` substitution also applies inside
  `secrets.akhq.security.basic-auth[].password`, only confirmed so far for `sasl.jaas.config` —
  and `charts/notification-worker/templates/externalsecret.yaml` (gated by new
  `externalSecret.enabled`, `secret.create` flipped to `false` — that chart's own
  `templates/secret.yaml`/`values.schema.json` already anticipated exactly this "externally
  provided secret" scenario before this work started). `charts/akhq/templates/basic-auth-secret.yaml`'s
  `target.template` field (`sha256sum`) needs ESO's own Go-template syntax protected from Helm's
  templating via `{{ "{{" }}`/`{{ "}}" }}` — verified correct by rendering and confirming the
  literal `{{ .password | sha256sum }}` survives into the rendered `ExternalSecret` unresolved.
  `kafka-akhq-user-secret.yaml` (the original chart-owned pre-supplied-password Secret) was
  **deleted outright on 2026-09-23**, not migrated — see the AKHQ-unification paragraph below.

  Real bug caught and fixed during implementation: the upstream `openbao` chart's
  `server.image.repository` must NOT include the registry (`registry`/`repository` are separate
  fields, unlike `notification-worker`'s own `image.repository` convention) — an initial
  `repository: quay.io/openbao/openbao` rendered as `quay.io/quay.io/openbao/openbao`, caught by
  actually grepping the rendered StatefulSet image line rather than trusting `helm lint`'s exit
  code (same "lint proves it renders, not that a value reached the right field" lesson already
  documented for AKHQ's `topologySpreadConstraints` under `charts/README.md`).

  **Second real bug, caught live (2026-09-23):** the chart's own `Chart.yaml` declares
  `appVersion: v2.6.2` (`v`-prefixed), and `charts/openbao/values.yaml`'s `server.image.tag` was
  set to match that literally (`"v2.6.2"`) — but the actual images published on
  `quay.io/openbao/openbao` are tagged **without** the `v` prefix (`2.6.2`, `2.6.1`, `2.6.0`,
  `latest`, …, confirmed via `quay.io/api/v1/repository/openbao/openbao/tag/`). `ImagePullBackOff`
  with `not found` on a real sync until fixed — the same tag mismatch was hardcoded in three
  places in that file (`server.image.tag`, the `auto-unseal` sidecar's `image:`, and
  `bootstrapJob.image`), all three now `"2.6.2"`. Lesson: an upstream chart's `AppVersion` field is
  a Helm/chart-metadata convention, not a guarantee that the literal string is also the real
  registry tag — check the actual registry (or just try a real pull) before hardcoding a version
  string copied from `Chart.yaml`, the same class of gap as the Strimzi
  `kafka.strimzi.io/v1beta2`-vs-`v1` API-version surprise documented elsewhere in this file.

  **Not yet live-verified:** the exact upstream Service names (`openbao-active`/`openbao-internal`)
  were confirmed against a real `helm template` render, but real HA failover between already-
  unsealed standbys, the auto-unseal sidecar actually firing against a genuinely restarted pod,
  and `ExternalSecret`s actually reaching `SecretSynced` all need a live cluster.

  **Extended 2026-09-23 with `PushSecret` (ESO's other direction — K8s → vault, `external-
  secrets.io/v1alpha1`, a separate, still-`v1alpha1` CRD from `ExternalSecret`'s stable `v1`) so
  OpenBao mirrors operator-generated credentials too, not just the ones this repo originates
  itself** — makes OpenBao a genuine central audit surface (`bao kv get ...` shows every
  credential in the cluster, not only the chart-seeded ones), per the B1 Begründung's "zentrale
  Audit-Fläche" claim. Two additions, both `updatePolicy: IfNotExists` (the operator sets the
  value once and doesn't rotate it, so `Replace` would just be a no-op write on every sync):
  - `charts/kafka-cluster/templates/messaging/kafka-user-pushsecrets.yaml` — one `PushSecret` per
    `kafka.users` entry **without** a `passwordSecretName`. Reads the Strimzi
    User-Operator-generated Secret (named after the `KafkaUser`, same namespace) and mirrors
    `password`/`sasl.jaas.config` to `secret/kafka/<username>`. For
    `catalog-api`/`reservation-worker`/`notification-worker` the consuming charts'
    `secretKeyRef`s are unchanged — they still read the Strimzi secret directly, never the
    mirrored copy.
  - **AKHQ unified with the other three users, same day:** until this change, `akhq` was the one
    `kafka.users` entry with an explicit `passwordSecretName` (its `KafkaUser` lives in the app
    namespace like all four, but the AKHQ *pod* runs in namespace `kafka` and can't reach a
    same-namespace-only `secretKeyRef` there, so it needed a pre-supplied rather than
    Strimzi-generated password). Removing `passwordSecretName` from its `kafka.users` entry lets
    Strimzi generate `akhq`'s password exactly like the other three; the same
    `kafka-user-pushsecrets.yaml` loop (already gated on "no `passwordSecretName`") picks it up
    automatically and mirrors it to `secret/kafka/akhq` — no special-casing left. For AKHQ, unlike
    the other three, the OpenBao round-trip **is** the real namespace bridge, not just an audit
    mirror: `charts/akhq/templates/kafka-sasl-secret.yaml` (`ExternalSecret`) pulls
    `sasl.jaas.config` back out of `secret/kafka/akhq` verbatim — Strimzi already generates a
    ready-to-use JAAS string (confirmed: same key shape the other three services already consume
    directly), so this ExternalSecret no longer reconstructs the string itself and no longer needs
    the Helm/ESO template-escaping trick (`{{ "{{" }}`/`{{ "}}" }}`) that a hand-built JAAS
    template required. This also deleted
    `charts/kafka-cluster/templates/messaging/kafka-akhq-user-secret.yaml`, the standalone
    `kafka.akhqUser` values block, and `charts/akhq/values.yaml`'s now-unused `kafkaSasl:` block
    entirely — one less special case, one less place two values had to be kept in sync by hand.
  - `charts/postgres-cluster` — `bootstrap.initdb` no longer pre-supplies a `secret:` (same fix
    already applied to `charts/openbao-postgres`), so CNPG now generates the ticketflow-app DB
    credential itself (`<clusterName>-app` — `postgres-cluster.appSecretName` in
    `_helpers.tpl`). New `templates/postgres-user-pushsecret.yaml` mirrors it to
    `secret/postgres/app-user`; the existing `templates/postgres-user-secret.yaml`
    (`ExternalSecret`) pulls it back out under the original, conventional name
    (`postgres-cluster.userSecretName`) so `catalog-api`/`reservation-worker` need zero changes.
    This also means the OpenBao bootstrap job **no longer invents this password itself** — the
    `seed_if_missing secret/postgres/app-user ...` line was removed from
    `charts/openbao/templates/bootstrap-job.yaml`; CNPG is now the sole origin of that value.
  - **Deliberately excluded:** `charts/openbao-postgres`'s own CNPG-generated credential
    (OpenBao's storage-backend password) is never pushed into OpenBao — circular by construction,
    OpenBao can't hold its own bootstrap credential before it's unsealed using that exact
    credential to reach its storage in the first place.
  - **Policy widened accordingly:** the bootstrap job's `external-secrets` Vault policy went from
    read-only (`path "secret/data/*" { capabilities = ["read"] }`) to
    `create`/`read`/`update` on both `secret/data/*` **and** `secret/metadata/*` — per
    external-secrets.io's own Vault-provider docs, `PushSecret` against a KV v2 mount needs write
    access to both, not just `data`.
  - Re-verified after this change: `helm lint`/`helm template` clean across the whole repo;
    rendered the new `PushSecret`s and confirmed `selector.secret.name` targets the real
    Strimzi/CNPG secret names and `remoteRef.remoteKey` lands on the same KV paths the
    corresponding `ExternalSecret`/bootstrap-job reads already use. **Not yet live-verified:**
    whether the Vault-compatible provider actually accepts the push against a real OpenBao
    instance (confirmed only against ESO's own documentation, not a running vault).

  **UI exposed publicly (2026-09-23), `helm lint`/`template` clean, not yet live-verified:**
  `charts/openbao/templates/ingress.yaml` — same pattern as `charts/akhq/templates/ingress.yaml`
  (own template, not the upstream chart's `ui.serviceType`/ingress fields, because the hostname
  hangs off the central `ticketflow.dnsZone` and Helm doesn't template `values.yaml`), plain
  `Ingress` at `openbao.<dnsZone>`, `cert-manager.io/cluster-issuer` annotation, Traefik
  terminates TLS. Backend is deliberately the `openbao-active` Service (port `http`, 8200), not
  `openbao`/`openbao-standby` — it always points at the current leader via
  `service_registration "kubernetes"`, independent of Kubernetes readiness
  (`publishNotReadyAddresses: true`, same reasoning as the readinessProbe override above); a
  standby must not receive UI traffic directly. `ui = true` was already set in the HCL
  `server.ha.config` block, no change needed there. **Deliberately no Traefik basic-auth in
  front, unlike a first instinct given how sensitive this UI is:** OpenBao has no
  human-usable auth method configured (no userpass/LDAP) — only the Shamir root token (readable
  solely via `kubectl get secret openbao-unseal-keys`, i.e. already requires cluster access) and
  Kubernetes-auth (ServiceAccounts only). A public Ingress therefore adds no real attack surface
  beyond the login screen itself; a second password there would be security theater, not a real
  gate. New `ingress:` values block (`className`/`clusterIssuerName`/`tlsSecretName`), same shape
  as `charts/akhq/values.yaml`.
- Commit history currently has a single large commit — the assignment explicitly penalizes
  "Monolith-Commits"; keep new work in small, separately-committed steps from here on.
