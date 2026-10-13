# infra/ansible

Konfiguriert die VMs, die von [`../terraform/`](../terraform/) erzeugt wurden: installiert k3s
(Server auf der Control Plane, Agents auf den Worker-Nodes), setzt die Node-Labels für die
simulierten Regionen und taintet die Control Plane (P4), bootstraps ArgoCD und rollt die
ApplicationSets stufenweise aus. Danach übernimmt [`../../gitops/`](../../gitops/) die
Konfiguration auf Kubernetes-Seite.

## Versionen

Bewusst fest gepinnt statt „latest“/„stable“ Tags zu verwenden (Reproduzierbarkeit – das P3-
Prinzip „keine `:latest`-Tags“ gilt hier ebenfalls). Zum Aktualisieren wird
[`inventory/group_vars/all.yml`](inventory/group_vars/all.yml) angepasst:

| Komponente | Version | Quelle |
|---|---|---|
| k3s | `v1.37.0+k3s1` | [k3s-io/k3s Releases](https://github.com/k3s-io/k3s/releases) |
| ArgoCD | `v3.4.5` | [argoproj/argo-cd Releases](https://github.com/argoproj/argo-cd/releases) (latest stable, nicht die 3.5 Release Candidates) |

## IPv6 (DHBWV6) und Dual-Stack

Trotz des Namens ist `DHBWV6` **dual-stack**: Zusätzlich zur IPv6-Adresse bekommt jede VM über
DHCP eine private, aber tatsächlich internet-routbare IPv4-Adresse (siehe [`../terraform/`](../terraform/)).
Ursprünglich wurde sie fälschlich als rein IPv6-only angenommen, was sich auf einem laufenden
Cluster als falsch erwies (die VMs erreichen `github.com`, das keinen `AAAA`-Eintrag hat, problemlos
per IPv4). IPv6 bleibt überall primär; IPv4 wird nur hinzugefügt, damit k3s dual-stack laufen
kann und Pods (ArgoCD, external-dns) IPv4-only-Ziele erreichen können. Für `roles/k3s_server`
heißt das:

- `--node-ip`/`--cluster-cidr`/`--service-cidr` erhalten jeweils ein `<IPv6>,<IPv4>`-Paar
  (IPv6 zuerst); `--tls-san` bekommt beide Adressen als separate Flags
- Die IPv6-CIDRs bleiben im ULA-Bereich `fd00::/8` (RFC 4193) statt eines global routbaren
  Blocks – Pods/Services müssen nicht von außen erreichbar sein; die IPv4-CIDRs sind die
  Standardwerte von k3s (`10.42.0.0/16`/`10.43.0.0/16`) und kollidieren nicht mit dem DHCP-
  Bereich der VMs `10.200.0.0/19`
- `--flannel-ipv6-masq`, damit IPv6-Pod-Egress hinter der Node-Adresse masqueraded wird
  (ULA ist nicht global routbar); k3s/Flannel masqueraden IPv4-Pod-Egress standardmäßig,
  dafür ist kein äquivalentes Flag nötig
- Die lokale `kubeconfig`, die k3s erzeugt, zeigt standardmäßig auf `127.0.0.1`/`[::1]` (IPv6
  zuerst in `--node-ip`, siehe oben); nach dem Download ersetzt `k3s_server` das mit
  `https://[<ipv6>]:6443` (Klammer-Syntax, sonst kollidiert das Doppelpunkt-`:` nach der Port
  Nummer mit den Adress-Doppelpunkten) – Kubeconfig/SSH bleiben bewusst IPv6-only; nur k3s selbst
  läuft dual-stack

## Struktur

```
ansible/
├── ansible.cfg
├── secrets.yml.example       # Vorlage für secrets.yml (gitignored, siehe unten)
├── inventory/
│   ├── group_vars/
│   │   └── all.yml          # gepinnte Versionen, IPv6-/IPv4-CIDRs
│   ├── hosts.ini             # von Terraform erzeugt, gitignored (siehe hosts.ini.example)
│   └── hosts.ini.example
├── playbooks/
│   ├── site.yml             # vollständiger Lauf: common -> k3s_server -> k3s_agent -> node_labels -> argocd_bootstrap -> gitops_secrets -> gitops_rollout
│   └── bootstrap-gitops.yml # nur argocd_bootstrap + gitops_secrets + gitops_rollout, separat wiederholbar
└── roles/
    ├── common/               # Paketindex, Basispakete, deterministischer Hostname, Multipathd-Exclusion für Longhorn
    ├── k3s_server/           # k3s-Server (Dual-Stack-Flags), Node-Token, lokales Laden der Kubeconfig
    ├── k3s_agent/            # k3s-Agent, Join über Token/URL der Control Plane
    ├── node_labels/          # topology.ticketflow/region je Worker + NoSchedule-Taint auf der Control Plane
    ├── argocd_bootstrap/     # Namespace, gepinnte Manifest-Versionen, Admin-Passwort
    ├── gitops_secrets/       # ArgoCD-Repo-Credential + RFC2136-TSIG-Kubernetes-Secrets (aus secrets.yml)
    └── gitops_rollout/       # ApplicationSets in Stufen: platform anwenden, auf Synced/Healthy warten, danach apps
```

## Warum Ansible und nicht alles in Terraform (`user_data`/cloud-init)

Terraform modelliert den *realen Zustand der Ressourcen*; wiederkehrende Konfigurationen (z. B.
Wiederholen von k3s nach einem manuellen Node-Reboot oder nur Ausführen der `node_labels`-Role)
sind mit einem dedizierten Konfigurationswerkzeug einfacher und idempotent, ohne den Terraform-
State anzufassen. Das entspricht auch genau der Vorgabe der Aufgabenstellung (Terraform/OpenTofu +
Ansible).

## Voraussetzungen

Ansible wird nicht systemweit erwartet; es läuft aus einer Python-venv (manche Debian/Ubuntu-
bzw. WSL-Images bringen systemweit kein `pip` mit, und `apt install` bräuchte Root-Rechte – eine
venv braucht das nicht). Ansible läuft nur unter Linux/WSL als Controller, nicht unter nativem
Windows (`ansible-core` scheitert dort mit `WinError 87`). Die venv wird einmal eingerichtet; die venv liegt bewusst im Projektstamm (`../../.venv`, bereits in
`.gitignore`), nicht unter `infra/ansible/`, damit andere Tools (Terraform-Helper, Python-Skripte)
sie projektweit gemeinsam nutzen können:

```bash
cd /path/to/repo-root          # DHBWCASCloudNative/
python3 -m venv .venv
source .venv/bin/activate
pip install -r infra/ansible/requirements.txt
```

Vor jedem Ansible-Aufruf aktivieren (`ansible-playbook`, `ansible-lint`, `ansible-inventory`, ...):

```bash
source /path/to/repo-root/.venv/bin/activate
```

Wie bei k3s/ArgoCD sind auch `ansible-core`/`ansible-lint` bewusst fest gepinnt
([`requirements.txt`](requirements.txt)); zum Aktualisieren muss dort angepasst werden, statt
`pip install -U` ohne Pin auszuführen. Das gilt nur für die Ansible-Control-Machine (WSL/Linux) – auf den verwalteten VMs selbst ist nichts Python-spezifisch nötig,
außer der System-Python-Version, die Cloud-Images ohnehin mitbringen und die Ansible per SSH
anspricht.

## Nutzung

```bash
cd infra/ansible
ansible-playbook playbooks/site.yml
```

Danach:

```bash
export KUBECONFIG=infra/ansible/kubeconfig
kubectl get nodes -o wide          # 4 Nodes, Worker mit Labels topology.ticketflow/region
kubectl -n argocd get pods         # ArgoCD läuft
kubectl -n argocd get applications -L ticketflow.io/stage   # 21 Applications, alle Synced/Healthy
```

Das ArgoCD-Admin-Passwort wird am Ende des Playbook-Laufs in der Konsolenausgabe ausgegeben
(`argocd_bootstrap` → Task „Show the initial admin password").

## GitOps-Secrets (`gitops_secrets`)

Zwei Dinge kann GitOps strukturell nicht selbst bereitstellen (ArgoCD ist git-getrieben, und
beide Werte sind Geheimnisse, die **nicht** im Git-Repo liegen dürfen, siehe Root-README ## Betrieb): die ArgoCD-
Repository-Credential für das private GitHub-Repo selbst sowie die RFC2136-TSIG-Kubernetes-
Secrets, auf die `charts/cert-manager-config`/`charts/external-dns` verweisen. Die Rolle
`gitops_secrets` schließt genau diese Lücken, läuft aber vollständig lokal gegen die abgerufene
`kubeconfig` (keine SSH, die Secrets landen nie in einer Prozessliste auf den VMs) und installiert
keine Helm-Charts/Operatoren selbst – das bleibt vollständig GitOps-eigen.

**Repo-Credential (SSH-Deploy-Key, einmalig):**

```bash
cd infra/ansible
ssh-keygen -t ed25519 -C "argocd-deploy-key@ticketflow" -f argocd_deploy_key -N ""
cat argocd_deploy_key.pub
```

Den ausgegebenen öffentlichen Schlüssel unter GitHub → Repo → Settings → Deploy keys → „Add deploy
key“ als **read-only**-Key hinterlegen. `argocd_deploy_key`/`argocd_deploy_key.pub` sind
gitignored. Jede von `gitops/applicationsets/` erzeugte `Application` zeigt auf ein Chart in
 diesem Repo (siehe `gitops/README.md` ## Struktur) und verwendet deshalb `repoURL:
 git@github.com:...` (SSH) statt `https://...` – nur die Upstream-Helm-Repos, auf die die
 Wrapper-Charts in ihren `Chart.yaml.dependencies` verweisen (Longhorn, cert-manager, external-dns,
 ...) bleiben normale öffentliche HTTPS-URLs und brauchen keine Credential.

Die Rolle liest die Repo-URL nicht mehr aus einem hartcodierten Literal, sondern über
`include_vars` aus `gitops/env/cluster.yaml` (`ticketflow.repoURL`) – derselben Datei, aus der
ArgoCD und alle Charts ihre Werte lesen. Das war früher hier ein Literal und damit genau die Stelle,
 an der ein Fork zuverlässig verpasst wurde: Die Credential wurden dann für das **Original-Repo**
registriert, und alle Applications blieben endlos `Unknown`, ohne dass die Fehlermeldung auf den
wirklichen Grund hingewiesen hat.

**RFC2136-TSIG (sobald die DNS-Zone aktiv ist):**

```bash
cp infra/ansible/secrets.yml.example infra/ansible/secrets.yml
# rfc2136_tsig_secret_value in secrets.yml eintragen (Datei ist gitignored)
```

Danach beide Schritte ausführen:

```bash
ansible-playbook playbooks/site.yml                    # oder gezielt:
ansible-playbook playbooks/bootstrap-gitops.yml         # nur ArgoCD + gitops_secrets + gitops_rollout erneut
```

Wenn `argocd_deploy_key` oder `secrets.yml` fehlt (oder letzteres noch den Placeholder `CHANGE_ME`
enthält), überspringt der jeweilige Teil der Rolle sich selbst statt zu failen – siehe
`gitops/README.md` ## DNS/ACME für den DNS-Teil der Gesamtübersicht. Ohne Repo-Credential kann
auch die Platform-Stage von `gitops_rollout` nie grün werden.

## Hinweise und bekannte Einschränkungen

- Keine eigene Ingress-/Load-Balancer-Logik: k3s installiert Traefik und ServiceLB standardmäßig;
  die Charts nutzen diese (Traefik wird über `charts/traefik-config` konfiguriert).
- `become` benötigt `sudo` ohne Passwort für den SSH-User, der in `hosts.ini` gesetzt ist – bei den
  üblichen Cloud-Images (z. B. Ubuntu-Cloud-Images mit vorbereitetem `ubuntu`-User) ist das der
  Standard.
- `ansible-lint` (Profil `basic`) meldet weiterhin rein stilistische Findings (FQCN-Modulnamen wie
  `command` statt `ansible.builtin.command`, `no-role-prefix` bei `register`-Variablen,
  fehlendes `changed_when` bei den gepufferten Install-Shells) – sie ändern das Verhalten nicht;
  absichtlich nicht behoben, um den Diff klein zu halten. Neue Roles (z. B. `gitops_secrets`)
  werden `risky-shell-pipe`-clean gehalten (`set -o pipefail`: ohne das meldete die
  `curl | sh`-Installation von k3s bei fehlgeschlagenem Download Erfolg, weil `sh` bei leerer
  Eingabe mit 0 endet). `ansible-lint` bei Bedarf erneut ausführen, um die aktuelle Liste zu sehen.
