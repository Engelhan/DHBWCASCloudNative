# infra/ansible

Konfiguriert die von [`../terraform/`](../terraform/) erzeugten VMs: installiert k3s (Server auf
der Control-Plane, Agents auf den Workern), setzt die Node-Labels für die simulierten Regionen
(P4) und bootstrappt ArgoCD. Ab dann übernimmt [`../../gitops/`](../../gitops/) die
Kubernetes-seitige Konfiguration.

## Versionen

Bewusst gepinnt statt "latest"/"stable"-Tags zu folgen (Reproduzierbarkeit, siehe P3-Prinzip
"keine `:latest`-Tags" auch hier angewendet). Zum Aktualisieren [`group_vars/all.yml`](group_vars/all.yml)
anpassen:

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
├── group_vars/
│   └── all.yml             # gepinnte Versionen, IPv6-CIDRs
├── inventory/
│   └── hosts.ini            # von Terraform generiert, git-ignoriert (siehe hosts.ini.example)
├── playbooks/
│   ├── site.yml             # Gesamtablauf: common -> k3s_server -> k3s_agent -> node_labels -> argocd_bootstrap
│   └── bootstrap-gitops.yml # nur der ArgoCD-Teil, separat erneut aufrufbar
└── roles/
    ├── common/               # Paketindex, Basispakete, deterministischer Hostname
    ├── k3s_server/           # k3s-Server (IPv6-Flags), Node-Token, kubeconfig lokal abholen
    ├── k3s_agent/            # k3s-Agent, Join per Token/URL der Control-Plane
    ├── node_labels/          # topology.ticketflow/region je Worker (aus dem Terraform-Inventory)
    └── argocd_bootstrap/     # Namespace, gepinntes Core-Manifest, Admin-Passwort, App-of-Apps
```

## Warum Ansible und nicht alles in Terraform (`user_data`/cloud-init)

Terraform bildet den *Ist-Zustand von Ressourcen* ab; wiederholtes Konfigurieren (z.B. erneutes
Ausrollen von k3s nach einem manuellen Node-Reboot, oder gezielt nur die `node_labels`-Rolle erneut
laufen lassen) ist mit einem eigenständigen Konfigurationswerkzeug einfacher und idempotent, ohne
Terraform-State anzufassen. Das ist auch die von der Aufgabenstellung vorgegebene Kombination
(Terraform/OpenTofu + Ansible).

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
