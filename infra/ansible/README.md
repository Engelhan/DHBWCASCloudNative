# infra/ansible

Konfiguriert die von [`../terraform/`](../terraform/) erzeugten VMs: installiert k3s (Server auf
der Control-Plane, Agents auf den Workern), setzt die Node-Labels für die simulierten Regionen
(P4), rollt Cluster-Addons aus (siehe ## Übernommene Funktionen aus k3s-dhbw-cloud-role unten) und
bootstrappt ArgoCD. Ab dann übernimmt [`../../gitops/`](../../gitops/) die Kubernetes-seitige
Konfiguration für die eigentliche TicketFlow-Anwendung.

## Übernommene Funktionen aus k3s-dhbw-cloud-role

Ein Teil der Rollen hier (`connectivity_check`, `helm_addons`, `cert_manager`, `external_dns`,
`longhorn`, `system_upgrade`, plus Ergänzungen in `common`/`k3s_server`/`k3s_agent`) ist
funktional aus [pfisterer/k3s-dhbw-cloud-role](https://github.com/pfisterer/k3s-dhbw-cloud-role)
übernommen, auf explizite Anweisung neu strukturiert in die bestehende rollenbasierte Gliederung
dieses Repos (statt der dortigen `NN_thema.yml`-Task-Datei-Nummerierung in einer einzigen Rolle).
Das ist eine **bewusste, dokumentierte Abweichung** von der ursprünglich in der Root-`CLAUDE.md`
festgehaltenen Grenze "Ansible macht nichts außer k3s/Labels/ArgoCD, alles Kubernetes-Level läuft
über gitops/" – hier explizit vom Nutzer so angefordert (inkl. der DNS/TLS-Automation).

Was NICHT übernommen wurde: die generische `ip_family`/Dual-Stack-Erkennung des Originals
(`03_network.yml`). Dieses Projekt ist bewusst und fest auf IPv6-only (DHBWV6) festgelegt (siehe
Terraform-Spezifika in der Root-`CLAUDE.md`) – die bestehenden festen IPv6-Werte in
`inventory/group_vars/all.yml` bleiben die einzige Quelle für Cluster-/Service-CIDR und Node-IP,
statt totes Dual-Stack-Codepfade mitzuschleppen, die in dieser Umgebung nie den IPv4-Zweig
nehmen können.

**Zwei bewusste Default-Abweichungen vom Original** (siehe Kommentare in
`inventory/group_vars/all.yml`):

- `k3s_auto_upgrade_enabled: false` (Original-Default: `true`) – widerspräche sonst dem
  projektweiten "keine `latest`-Tags, Versionen bewusst pinnen"-Prinzip und würde die
  Reproduzierbarkeit für die Abgabe gefährden. Die Rolle `system_upgrade` ist voll
  funktionsfähig, nur standardmäßig aus.
- `cert_manager_email` und `rfc2136_zone`/TSIG-Variablen bleiben leer – ohne eine echte, im
  Projekt registrierte DHBW-DNS-Zone (siehe [self-service.dhbw.cloud](https://self-service.dhbw.cloud))
  wären `cert_manager`/`external_dns` ohnehin nur mit Platzhalterwerten lauffähig. Beide Rollen
  prüfen das selbst (`when: ... is defined and ... != ""`) und überspringen sich sauber, statt mit
  einer kaputten ACME-Registrierung fehlzuschlagen. Zum Aktivieren die Variablen in
  `inventory/group_vars/all.yml` setzen (Kommentare dort zeigen wie).

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
├── requirements.txt         # pip: ansible, ansible-lint (Steuerungsmaschine)
├── requirements.yml         # ansible-galaxy: kubernetes.core (Addon-Rollen)
├── inventory/
│   ├── group_vars/
│   │   └── all.yml          # gepinnte Versionen, IPv6-CIDRs, Addon-Variablen
│   ├── hosts.ini             # von Terraform generiert, git-ignoriert (siehe hosts.ini.example)
│   └── hosts.ini.example
├── playbooks/
│   ├── site.yml             # Gesamtablauf, siehe Rollen-Reihenfolge unten
│   └── bootstrap-gitops.yml # nur der ArgoCD-Teil, separat erneut aufrufbar
└── roles/
    ├── common/               # Paketindex, Basispakete, Hostname, Kernel-Sysctls, Longhorn-Node-Prereqs
    ├── connectivity_check/   # VXLAN-Overlay-Preflight vor der k3s-Installation (alle Nodes)
    ├── k3s_server/           # k3s-Server (IPv6-Flags), Node-Token, kubeconfig, Client-Tooling
    ├── k3s_agent/            # k3s-Agent, Join per Token/URL der Control-Plane
    ├── node_labels/          # topology.ticketflow/region je Worker (aus dem Terraform-Inventory)
    ├── helm_addons/          # helm-diff-Plugin, Helm-Repos fuer die folgenden Addons
    ├── cert_manager/         # cert-manager + ClusterIssuer (aus, ohne cert_manager_email)
    ├── external_dns/         # external-dns via RFC2136 (aus, ohne rfc2136_zone)
    ├── longhorn/             # verteilter Block-Storage (an, Replica-Count 1)
    ├── system_upgrade/       # automatische k3s-Upgrades (AUS per Default, siehe oben)
    └── argocd_bootstrap/     # Namespace, gepinntes Core-Manifest, Admin-Passwort, App-of-Apps
```

Reihenfolge in `playbooks/site.yml`: `common` → `connectivity_check` → `k3s_server` →
`k3s_agent` → `node_labels` → `helm_addons` → `cert_manager` + `external_dns` + `longhorn` →
`system_upgrade` → `argocd_bootstrap`.

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
cd infra/ansible
ansible-galaxy collection install -r requirements.yml   # kubernetes.core fuer die Addon-Rollen
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
kubectl get storageclass           # longhorn (Default), local-path deaktiviert
kubectl -n longhorn-system get pods
kubectl -n argocd get pods         # ArgoCD läuft
```

Das ArgoCD-Admin-Passwort wird am Ende des Playbook-Laufs in der Konsolenausgabe angezeigt
(`argocd_bootstrap` → Task "Initiales Admin-Passwort anzeigen").

cert-manager und external-dns sind per Default aus (siehe ## Übernommene Funktionen aus
k3s-dhbw-cloud-role) – zum Aktivieren `cert_manager_email` bzw. `rfc2136_zone` +
`rfc2136_tsig_secret_*` in `inventory/group_vars/all.yml` setzen und
`ansible-playbook playbooks/site.yml` erneut laufen lassen (alle Rollen sind idempotent, nur die
neu aktivierten Addon-Tasks führen etwas aus).

## Offene Punkte

- `gitops/apps/app-of-apps.yaml` existiert noch nicht (siehe [`../../gitops/`](../../gitops/)) –
  `argocd_bootstrap` prüft das per `stat` und überspringt das Anwenden, statt fehlzuschlagen.
  Sobald die Datei geschrieben ist, holt `ansible-playbook playbooks/bootstrap-gitops.yml` sie
  nach.
- Kein eigenes Ingress/LoadBalancer-Handling: k3s installiert standardmäßig Traefik und
  ServiceLB mit. Ob das für die spätere Anwendung passt oder abgeschaltet wird, entscheidet sich
  beim Schreiben des Helm-Charts ([`../../charts/ticketflow/`](../../charts/ticketflow/)).
- `become` setzt passwortlosen `sudo` für den in `hosts.ini` hinterlegten SSH-User voraus – auf
  den üblichen Cloud-Images (z.B. Ubuntu-Cloud-Images mit vorbereitetem `ubuntu`-User) ist das
  der Standard.
- `ansible-lint` (Profil `basic`) meldet aktuell noch 119 rein stilistische Funde (FQCN-Modulnamen
  wie `command` statt `ansible.builtin.command`, `no-role-prefix` bei `register`-Variablen,
  Task-Namen mit Kleinbuchstaben wie "k3s-Server installieren", fehlendes `changed_when` bei den
  gated Install-Shells) – keine davon ändert das Verhalten, bewusst nicht mitgefixt, um den Diff
  klein zu halten. Bei Bedarf `ansible-lint` erneut laufen lassen, um die aktuelle Liste zu sehen.
- `cert_manager`/`external_dns` brauchen eine echte, im Projekt registrierte DHBW-DNS-Zone +
  TSIG-Key (siehe [self-service.dhbw.cloud](https://self-service.dhbw.cloud)) – ohne die bleiben
  beide Rollen inaktiv (siehe ## Übernommene Funktionen aus k3s-dhbw-cloud-role). Noch nicht gegen
  eine echte Zone getestet.
- Die Addon-Rollen (`helm_addons`, `cert_manager`, `external_dns`, `longhorn`, `system_upgrade`)
  sind wie der Rest statisch verifiziert (`--syntax-check`, `ansible-lint`, Jinja-Render-Checks der
  Templates/`combine()`-Ausdrücke), aber noch nicht gegen echte VMs gelaufen – anders als
  `common`/`k3s_server`/`k3s_agent`/`node_labels`/`argocd_bootstrap`, die das bereits sind (siehe
  Root-`CLAUDE.md`).
