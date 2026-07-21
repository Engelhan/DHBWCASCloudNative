# infra/ansible

Konfiguriert die von [`../terraform/`](../terraform/) erzeugten VMs: installiert k3s (Server auf
der Control-Plane, Agents auf den Workern), setzt die Node-Labels für die simulierten Regionen
(P4) und bootstrappt ArgoCD. Ab dann übernimmt [`../../gitops/`](../../gitops/) die
Kubernetes-seitige Konfiguration.

## Versionen

Bewusst gepinnt statt "latest"/"stable"-Tags zu folgen (Reproduzierbarkeit, siehe P3-Prinzip
"keine `:latest`-Tags" auch hier angewendet). Zum Aktualisieren
[`inventory/group_vars/all.yml`](inventory/group_vars/all.yml) anpassen:

| Komponente | Version | Quelle |
|---|---|---|
| k3s | `v1.36.2+k3s1` | [k3s-io/k3s Releases](https://github.com/k3s-io/k3s/releases) |
| ArgoCD | `v3.4.4` | [argoproj/argo-cd Releases](https://github.com/argoproj/argo-cd/releases) (neueste stabile Version, nicht die 3.5-Release-Candidates) |

## IPv6 (DHBWV6)

Das Cluster-Netz ist reines IPv6 (siehe [`../terraform/`](../terraform/)). Daraus folgt für
`roles/k3s_server`:

- `--node-ip`/`--tls-san` auf die eigene, von Terraform gelieferte IPv6-Adresse
- `--cluster-cidr`/`--service-cidr` als IPv6-Blöcke im ULA-Bereich `fd00::/8` (RFC 4193) statt
  eines global routbaren Blocks – Pods/Services müssen von außen nicht erreichbar sein
- `--flannel-ipv6-masq`, damit Pod-Egress-Traffic über die Node-Adresse maskiert wird
- Die von k3s erzeugte lokale `kubeconfig` verweist standardmäßig auf `127.0.0.1`; `k3s_server`
  biegt das nach dem Abholen auf `https://[<ipv6>]:6443` um (Klammer-Syntax, sonst kollidiert der
  Port-Doppelpunkt mit den Adress-Doppelpunkten)

## Struktur

```
ansible/
├── ansible.cfg
├── secrets.yml.example       # Vorlage fuer secrets.yml (git-ignoriert, siehe unten)
├── inventory/
│   ├── group_vars/
│   │   └── all.yml          # gepinnte Versionen, IPv6-CIDRs
│   ├── hosts.ini             # von Terraform generiert, git-ignoriert (siehe hosts.ini.example)
│   └── hosts.ini.example
├── playbooks/
│   ├── site.yml             # Gesamtablauf: common -> k3s_server -> k3s_agent -> node_labels -> argocd_bootstrap -> gitops_secrets
│   └── bootstrap-gitops.yml # nur ArgoCD/App-of-Apps + gitops_secrets, separat erneut aufrufbar
└── roles/
    ├── common/               # Paketindex, Basispakete, deterministischer Hostname
    ├── k3s_server/           # k3s-Server (IPv6-Flags), Node-Token, kubeconfig lokal abholen
    ├── k3s_agent/            # k3s-Agent, Join per Token/URL der Control-Plane
    ├── node_labels/          # topology.ticketflow/region je Worker (aus dem Terraform-Inventory)
    ├── argocd_bootstrap/     # Namespace, gepinntes Core-Manifest, Admin-Passwort, App-of-Apps
    └── gitops_secrets/       # RFC2136-TSIG-K8s-Secrets fuer cert-manager/external-dns (aus secrets.yml)
```

## Warum Ansible und nicht alles in Terraform (`user_data`/cloud-init)

Terraform bildet den *Ist-Zustand von Ressourcen* ab; wiederholtes Konfigurieren (z.B. erneutes
Ausrollen von k3s nach einem manuellen Node-Reboot, oder gezielt nur die `node_labels`-Rolle erneut
laufen lassen) ist mit einem eigenständigen Konfigurationswerkzeug einfacher und idempotent, ohne
Terraform-State anzufassen. Das ist auch die von der Aufgabenstellung vorgegebene Kombination
(Terraform/OpenTofu + Ansible).

## Voraussetzungen

Ansible ist nicht systemweit installiert vorausgesetzt, sondern läuft aus einem Python-venv (auf
diesem Debian/Ubuntu-System fehlt `pip` sogar im System-Python, und `apt install`
erfordert Root-Rechte, die im WSL-Setup nicht passwortlos verfügbar sind — ein venv braucht
keine der beiden). Einmalig einrichten, venv liegt bewusst im Projekt-Root (`../../.venv`,
dort schon in `.gitignore`), nicht unter `infra/ansible/`, damit es projektweit für andere
Tools (Terraform-Helper, Python-Skripte) mitbenutzt werden kann:

```bash
cd /pfad/zum/repo-root         # DHBWCASCloudNative/
python3 -m venv .venv
source .venv/bin/activate
pip install -r infra/ansible/requirements.txt
```

Vor jedem Ansible-Aufruf aktivieren (`ansible-playbook`, `ansible-lint`, `ansible-inventory`, ...):

```bash
source /pfad/zum/repo-root/.venv/bin/activate
```

`ansible-core`/`ansible-lint` sind wie k3s/ArgoCD bewusst version-gepinnt
([`requirements.txt`](requirements.txt)), zum Aktualisieren dort anpassen statt `pip install -U`
ohne Pin laufen zu lassen. Gilt nur für die Ansible-Steuerungsmaschine (WSL/Linux, siehe
Root-`CLAUDE.md`) — auf den verwalteten VMs selbst wird nichts Python-seitig vorausgesetzt außer
dem ohnehin auf Cloud-Images vorhandenen System-Python, das Ansible über SSH anspricht.

## Verwendung

```bash
cd infra/ansible
ansible-playbook playbooks/site.yml
```

Danach:

```bash
export KUBECONFIG=infra/ansible/kubeconfig
kubectl get nodes -o wide          # 4 Nodes, Worker mit Label topology.ticketflow/region
kubectl -n argocd get pods         # ArgoCD läuft
```

Das ArgoCD-Admin-Passwort wird am Ende des Playbook-Laufs in der Konsolenausgabe angezeigt
(`argocd_bootstrap` → Task "Initiales Admin-Passwort anzeigen").

## GitOps-Secrets (`gitops_secrets`)

Zwei Dinge, die GitOps strukturell nicht selbst bereitstellen kann (ArgoCD ist git-getrieben,
beide Werte dürfen laut `CLAUDE.md` ## Infra layering **nicht** im Git-Repo stehen): das
ArgoCD-Repository-Credential fürs private GitHub-Repo selbst, und die RFC2136-TSIG-K8s-Secrets,
die `gitops/apps/cert-manager-issuer.yaml`/`external-dns.yaml` referenzieren. Die Rolle
`gitops_secrets` schließt beide Lücken, läuft aber komplett lokal gegen die abgeholte
`kubeconfig` (kein SSH, die Geheimnisse landen nie in einer Prozessliste auf den VMs) und
installiert selbst keine Helm-Charts/Operatoren — das bleibt vollständig GitOps-eigen.

**Repo-Credential (SSH-Deploy-Key, einmalig):**

```bash
cd infra/ansible
ssh-keygen -t ed25519 -C "argocd-deploy-key@ticketflow" -f argocd_deploy_key -N ""
cat argocd_deploy_key.pub
```

Den ausgegebenen Public Key unter GitHub → Repo → Settings → Deploy keys → "Add deploy key" als
**Read-only**-Key eintragen. `argocd_deploy_key`/`argocd_deploy_key.pub` sind git-ignoriert.
`gitops/apps/app-of-apps.yaml` und `cert-manager-issuer.yaml` nutzen deshalb bewusst
`repoURL: git@github.com:...` (SSH) statt `https://...` — alle Applications, die stattdessen
einen fremden Upstream-Helm-Repo referenzieren (Longhorn, cert-manager, external-dns), bleiben
unverändert HTTPS, die brauchen kein Credential.

**RFC2136-TSIG (sobald die DNS-Zone aktiv ist):**

```bash
cp infra/ansible/secrets.yml.example infra/ansible/secrets.yml
# rfc2136_tsig_secret_value in secrets.yml eintragen (Datei ist git-ignoriert)
```

Danach beides anwenden:

```bash
ansible-playbook playbooks/site.yml                    # oder gezielt:
ansible-playbook playbooks/bootstrap-gitops.yml         # nur ArgoCD + gitops_secrets erneut
```

Fehlt `argocd_deploy_key` bzw. `secrets.yml` (oder enthält sie noch den Platzhalter
`CHANGE_ME`), überspringt sich der jeweilige Teil der Rolle selbst statt fehlzuschlagen — siehe
`gitops/README.md` ## DNS/ACME für den DNS-Teil des Gesamtkontexts.

## Offene Punkte

- Kein eigenes Ingress/LoadBalancer-Handling: k3s installiert standardmäßig Traefik und
  ServiceLB mit. Ob das für die spätere Anwendung passt oder abgeschaltet wird, entscheidet sich
  beim Schreiben des Helm-Charts ([`../../charts/ticketflow/`](../../charts/ticketflow/)).
- `become` setzt passwortlosen `sudo` für den in `hosts.ini` hinterlegten SSH-User voraus – auf
  den üblichen Cloud-Images (z.B. Ubuntu-Cloud-Images mit vorbereitetem `ubuntu`-User) ist das
  der Standard.
- `ansible-lint` (Profil `basic`) meldet weiterhin rein stilistische Funde (FQCN-Modulnamen wie
  `command` statt `ansible.builtin.command`, `no-role-prefix` bei `register`-Variablen,
  Task-Namen mit Kleinbuchstaben wie "k3s-Server installieren", fehlendes `changed_when` bei den
  gated Install-Shells) – keine davon ändert das Verhalten, bewusst nicht mitgefixt, um den Diff
  klein zu halten. Neue Rollen (z.B. `gitops_secrets`) werden dagegen `risky-shell-pipe`-sauber
  gehalten (`set -o pipefail`, siehe CLAUDE.md-Hinweis zum `curl | sh`-Bug). Bei Bedarf
  `ansible-lint` erneut laufen lassen, um die aktuelle Liste zu sehen.
