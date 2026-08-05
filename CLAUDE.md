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
   (CNPG, Strimzi, KEDA), the observability stack (kube-prometheus-stack, Loki, Tempo), cluster
   addons (Longhorn, cert-manager, external-dns), and the app chart itself, as GitOps-synced
   `Application` resources. Every `Application` points at a chart under `charts/` in this repo
   (never directly at an upstream Helm repo) — for addons that exist upstream as a chart, that's
   a small wrapper chart whose `Chart.yaml` declares the upstream chart as a `dependencies`
   entry, with our values in its own `values.yaml` (see `charts/README.md`); ArgoCD resolves the
   dependency via `helm dependency build` at sync time. `app-of-apps.yaml` +
   Longhorn/cert-manager/cert-manager-issuer/external-dns/argocd-ingress/cnpg-operator/
   strimzi-operator/kube-prometheus-stack/loki/tempo are written; KEDA and `ticketflow.yaml` are
   still **not implemented** (build them the same wrapper-chart way). CNPG/Strimzi/
   kube-prometheus-stack all bundle CRDs over the 262144-byte annotation limit (CNPG's `poolers`
   CRD >650KB, Strimzi's `Kafka` CRD >330KB, kube-prometheus-stack's `Prometheus` CRD >800KB) —
   their Applications need `syncOptions: [ServerSideApply=true]`, same fix as the ArgoCD install
   itself (see Ansible specifics below); apply this to KEDA too if its CRDs turn out large. Loki/
   Tempo ship no CRDs, don't need it. Observability persistence goes through Longhorn
   throughout (already the default StorageClass); Loki runs `SingleBinary` mode with a bundled
   MinIO as its S3-compatible chunk store, Tempo uses its local filesystem backend — both are
   PVC-backed. Grafana (from kube-prometheus-stack) gets Loki + Tempo wired in as
   `additionalDataSources`, one Grafana for the full LGTM stack. See `gitops/README.md` ##
   Observability-Stack.
   See
   `gitops/README.md` for the DNS/ACME activation flow.
4. **`charts/ticketflow/`** — the umbrella Helm chart for the app layer (services, CNPG
   `Cluster` CR, Strimzi `Kafka` CR, ServiceMonitors, KEDA `ScaledObject`). Currently only a
   README — **no templates written yet**.

Terraform → Ansible → ArgoCD → Helm chart is a hard boundary an agent should preserve: e.g. don't
add k3s app deployments via Ansible, don't add OpenStack resources via a Helm hook, etc.

**This boundary was briefly crossed and then reverted.** Branch `ansible-bootstrap` (commit
`938a66f`) ports cluster addons (Longhorn, cert-manager, external-dns, auto-upgrade,
connectivity-check preflight) from
[pfisterer/k3s-dhbw-cloud-role](https://github.com/pfisterer/k3s-dhbw-cloud-role) as Ansible
roles that call `kubernetes.core.helm` directly — a deliberate exception at the time. That
approach was superseded on `main` by deploying the same addons (Longhorn, cert-manager,
external-dns) through `gitops/apps/` instead (see `gitops/README.md`), restoring the boundary
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
  control-plane lab cluster) and applies `gitops/apps/app-of-apps.yaml` **only if that file
  already exists** (checked via `stat`, skips otherwise instead of failing) — the file now exists
  (see gitops layer above), so this actually runs; the `stat` guard stays as a safety net for
  older checkouts/tags without it.
- `gitops_secrets` (new role) creates two things GitOps structurally can't provide itself: the
  ArgoCD repository credential for this repo (**this repo is private** — every `Application` in
  `gitops/apps/` uses `repoURL: git@github.com:...` SSH, not `https://`, since every one of them
  points at a chart under `charts/` in this repo, not directly at an upstream Helm repo — see
  Infra layering above; only the upstream Helm repos referenced from within each wrapper chart's
  `Chart.yaml.dependencies` need no credential, ArgoCD builds those via `helm dependency build`)
  and the RFC2136-TSIG `Secret`s that `cert-manager-issuer.yaml`/`external-dns.yaml` reference via
  `secretKeyRef`. This is a narrow, deliberate exception to "Ansible does nothing beyond
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
- Pinned versions in `inventory/group_vars/all.yml`: k3s `v1.36.2+k3s1`, ArgoCD `v3.4.5` (latest
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
# Fork personalization (only when deploying under a different DHBW account/repo — skip for
# work on the original repo). Rewrites the non-secret, git-tracked identity values (fork
# repoURL, DNS zone, ACME email, TSIG keyname) across gitops/**/*.yaml + charts/**/*.yaml in
# one pass instead of hand-editing ~9 files. Secrets (TSIG value, OpenStack creds) are
# untouched by this — those still go through terraform.tfvars/secrets.yml as usual.
cp deploy.env.example deploy.env   # fill in real values, or set as env vars instead
python3 scripts/personalize.py
git diff                           # review before committing

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
- **Done, verified live on a real (rebuilt, dual-stack) cluster:** `gitops/apps/app-of-apps.yaml`,
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
  That hostname is published by `gitops/apps/traefik-config.yaml` + `charts/traefik-config/`, a
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
  (`cas438007-at-cas-dhbw-de.users.dhbw.site`) and `syncPolicy.automated`. The RFC2136 TSIG
  secret itself is deliberately **not** in the repo — `infra/ansible/roles/gitops_secrets`
  creates the K8s `Secret`s it needs from a gitignored local `infra/ansible/secrets.yml`
  (template: `secrets.yml.example`), run automatically at the end of `playbooks/site.yml` (see
  Ansible specifics above and `infra/ansible/README.md` ## GitOps-Secrets). `app-of-apps.yaml`
  excludes itself from its own directory source (`source.directory.exclude`) — without it,
  self-tracking + `prune: true` risked ArgoCD deleting its own root Application (and cascading
  through the `resources-finalizer` to every child app); see `gitops/README.md` ## Struktur.
- **Done:** `scripts/personalize.py` + `deploy.env.example` — lets another student/the professor
  deploy their own fork by rewriting the git-tracked, non-secret identity values (fork repoURL,
  DNS zone, ACME email, TSIG keyname) across `gitops/**/*.yaml` + `charts/**/*.yaml` in one pass
  instead of hand-editing ~9 files; supports env-var overrides too (values are read from the repo
  itself at runtime via `CANONICAL_SOURCES`, never hardcoded as literals in the script). Secrets
  are deliberately out of scope (stay in `terraform.tfvars`/`secrets.yml` as before). See root
  `README.md` ## Betrieb ### Eigenes Deployment.
- **Committed but not yet re-verified live** (check `kubectl -n argocd get applications` before
  assuming any of this is synced clean — several commits landed after the last confirmed-live
  check): the Longhorn/cert-manager/external-dns wrapper-chart restructure, the ArgoCD/Longhorn/
  cert-manager/external-dns version bumps (latest stable as of this writing), and five new
  wrapper charts + Applications:
  - `gitops/apps/cnpg-operator.yaml` (`charts/cnpg-operator/`, CloudNativePG `v1.30.0`/chart
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
  `charts/README.md`, first chart in the repo with `_helpers.tpl` since it renders 6 resources:
  Deployment/Service/ConfigMap/Secret/PodDisruptionBudget/ServiceMonitor, plus an optional
  NetworkPolicy), `replicaCount: 3` + `PodDisruptionBudget` by default, and its own
  `values.schema.json` (the only per-service chart in the repo with one so far — everywhere else
  a schema is tied to the not-yet-started umbrella chart). No `gitops/apps/notification-worker.yaml`
  yet (out of scope for now — chart is lint/template-only, not deployed via ArgoCD). `go.sum` is
  not yet generated (no `go`/`docker`/`helm` toolchain available in the environment that wrote
  this code) — run `go mod tidy` before the first real build.
- **Not started:** `gitops/apps/keda.yaml`, `ticketflow.yaml`,
  `gitops/projects/ticketflow-project.yaml`, `charts/ticketflow/templates/*`, remaining `src/*`
  application code (`catalog-api`, `reservation-worker`, `frontend`, `load-generator`),
  `observability/dashboards/` + `observability/alerts/` content.
- Commit history currently has a single large commit — the assignment explicitly penalizes
  "Monolith-Commits"; keep new work in small, separately-committed steps from here on.
