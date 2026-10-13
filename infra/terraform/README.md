# infra/terraform

Hier wird nur die Ressourcenebene auf DHBWCloud über den **OpenStack Provider**
(`terraform-provider-openstack/openstack`) provisioniert. Kubernetes und App-Konfigurationen
werden hier bewusst nicht verwaltet – das übernimmt [`../ansible/`](../ansible/). Es gibt
absichtlich **nur eine** Umgebung: In der Laborumgebung gibt es keinen produktiven Parallelbetrieb,
weshalb ein zweiter Terraform-Workspace bzw. ein zweiter Verzeichnisbaum keinen sinnvollen Nutzen
brächte. (Die Dateien `values-dev.yaml`/`values-prod.yaml` beziehen sich auf die Helm-Charts, nicht
auf die Infrastruktur.)

## Ressourcen

- Kein eigenes Netzwerk, keine Subnetze, kein eigener Router und keine selbst verwaltete
  Security Group: DHBWCloud stellt bereits **DHBWV6** bereit, ein extern routbares
  Projekt-Netzwerk inklusive Default-Security-Group. `main.tf` liest dieses Netzwerk nur per
  Data Source aus und erzeugt nichts davon neu.
- Ein SSH-Keypair (importierter öffentlicher Schlüssel)
- VMs ([`modules/compute`](modules/compute/)), die direkt an DHBWV6 angebunden sind und jeweils
  eine direkt erreichbare `access_ip_v6` (primäre Adresse für SSH/Kubeconfig) **sowie** eine
  per DHCP zugewiesene, tatsächlich internet-routbare `access_ip_v4` erhalten (**DHBWV6 ist
  trotz des Namens dual-stack**, nicht nur IPv6 – Terraform gibt `access_ip_v4` nur zusätzlich
  weiter, damit k3s dual-stack laufen kann, siehe [`../ansible/README.md`](../ansible/README.md)
  unter „IPv6 (DHBWV6) und Dual-Stack"):
  - 1× `control-plane` (k3s Server)
  - 1× Worker pro Eintrag in `var.worker_regions` (Standardwert 3: `region-a`, `region-b`,
    `region-c`), alle mit derselben Flavor
- Eine erzeugte `infra/ansible/inventory/hosts.ini` (`local_file`-Ressource in `main.tf`, aus
  [`templates/hosts.ini.tpl`](templates/hosts.ini.tpl)) – damit ist der Schritt
  „Terraform Output → Ansible Inventory“ automatisiert und wird direkt mit `terraform apply`
  ausgeführt, statt die IPs manuell zu kopieren.

## Warum „Regions“ statt eines storage-optimierten Workers

DHBWCloud bietet in der Praxis wirklich nur eine Region/eine AZ. Ein Unterschied zwischen
Flavors wie „mehr/schnellerer Speicher“ wäre nur eine künstliche Platzierungsregel aus der
Vorlesung und kein echter Infrastruktur-Parameter. Deshalb bekommt jeder Worker über Metadaten
ein virtuell simuliertes Regions-Label (`region = "region-a"` usw.). Genau darauf basiert die
eigentliche Platzierungsentscheidung für die Domäne (P4): CNPG-Replikate, Kafka-Broker und alle
Dienste, die doppelt laufen, werden gezielt über unterschiedliche simulierte Regionen verteilt
(TopologySpread/Anti-Affinity auf dem Kubernetes-Node-Label `topology.ticketflow/region`), damit
 ein einzelner Node/Region-Fehler nicht alle Replikate gleichzeitig trifft – siehe
[`../ansible/roles/`](../ansible/roles/) sowie die Root-README unter `## Entscheidungen`.

4 VMs insgesamt, davon 3 Worker – erfüllt die Anforderung P6 („mehrere Worker, damit Skalierung
und Verteilung sichtbar werden").

## Struktur

```
terraform/
├── modules/
│   └── compute/   # Keypair, Control-Plane-/Worker-VMs (direkt auf DHBWV6, Regions-Metadaten)
├── templates/
│   └── hosts.ini.tpl   # Template für das Ansible-Inventory
├── main.tf              # DHBWV6-Data-Source, Modulverkabelung, Inventar-Erzeugung
├── variables.tf
├── outputs.tf
├── providers.tf
├── versions.tf
└── terraform.tfvars.example
```

## Nutzung

```bash
cd infra/terraform
cp terraform.tfvars.example terraform.tfvars   # OpenStack-Zugang, Flavor-/Image-Namen eintragen
terraform init
terraform plan
terraform apply
```

Nach `terraform apply` existiert bereits `infra/ansible/inventory/hosts.ini`; als Nächstes folgt
`ansible-playbook -i inventory/hosts.ini playbooks/site.yml` in [`../ansible/`](../ansible/).

## offene Punkte

- `image_name`, `control_plane_flavor`, `worker_flavor` sind DHBWCloud-spezifisch und haben
  bewusst **keinen** Standardwert – sie müssen aus dem DHBWCloud-Portal bzw. der
  Dokumentation übernommen werden. `network_name` defaultet auf `DHBWV6`, `worker_regions` auf
  drei simulierte Regionen.
- Da keine eigene Security Group verwaltet wird, gilt die Default-Security-Group des DHBWCloud-
  Projekts. Wenn diese nicht bereits SSH/k3s-API/NodePort erlaubt, muss sie einmalig außerhalb
  dieses Terraform-Codes konfiguriert werden, denn die Vorgaben verlangen explizit keine
  Security-Group-Verwaltung hier.
- IPv6-Primär: `ssh`/`ansible` erreichen die VMs über `access_ip_v6`; das erzeugte `hosts.ini`
  trägt die IPv6-Adresse als `ansible_host` und zusätzlich `access_ip_v4` als `node_ip_v4` (eigenes
  Host-Var, siehe `hosts.ini.tpl`). k3s läuft dual-stack in [`../ansible/`](../ansible/)
  (`--node-ip`/`--cluster-cidr`/`--service-cidr` mit IPv6 zuerst, IPv4 zusätzlich – siehe dort
  die README unter „IPv6 (DHBWV6) und Dual-Stack“). Die Kubeconfig-Server-URL bleibt IPv6-only in
  Klammer-Schreibweise `https://[<ipv6>]:6443` (siehe `outputs.tf` → `kubeconfig_hint`).
