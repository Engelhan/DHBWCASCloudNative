# infra/terraform

Provisioniert die reine Ressourcenlage auf der DHBWCloud gegen den **OpenStack-Provider**
(`terraform-provider-openstack/openstack`). Kein Kubernetes, keine App-Konfiguration hier – das
übernimmt [`../ansible/`](../ansible/). Es gibt bewusst **eine** Umgebung: das Lab hat keinen
produktiven Parallelbetrieb, ein zweiter Terraform-Workspace/Verzeichnisbaum wäre nur
unbegründeter Zusatzaufwand. (Die geforderten `values-dev.yaml`/`values-prod.yaml` aus W1 beziehen
sich auf den Helm-Chart, nicht auf die Infrastruktur.)

## Ressourcen

- Kein eigenes Netz/Subnetz/Router und keine selbst verwaltete Security Group: die DHBWCloud
  stellt mit **DHBWV6** bereits ein extern routbares Projektnetz samt Default-Security-Group
  bereit. `main.tf` sucht dieses Netz nur per Data-Source, legt nichts neu an.
- Ein SSH-Keypair (importierter öffentlicher Schlüssel)
- VMs ([`modules/compute`](modules/compute/)), jede direkt an DHBWV6 mit unmittelbar
  erreichbarer `access_ip_v6` (**DHBWV6 ist ein reines IPv6-Netz**, die VMs bekommen keine
  IPv4-Adresse):
  - 1× `control-plane` (k3s-Server)
  - 1× Worker je Eintrag in `var.worker_regions` (Default 3: `region-a`, `region-b`, `region-c`),
    alle mit demselben Flavor
- Ein generiertes `infra/ansible/inventory/hosts.ini` (`local_file`-Resource in `main.tf`, aus
  [`templates/hosts.ini.tpl`](templates/hosts.ini.tpl)) – automatisiert genau den Schritt
  "Terraform-Output → Ansible-Inventory" und macht ihn Teil von `terraform apply`, statt IPs
  manuell zu übertragen.

## Warum "Regionen" statt einem storage-optimized Worker

Die DHBWCloud stellt real nur eine Region/AZ bereit; ein Flavor-Unterschied wie "mehr/schnellerer
Storage" wäre nur eine künstliche, aus der Vorlesung übernommene Platzierungsregel gewesen. Statt
dessen bekommt jeder Worker per Metadata (`region = "region-a"` usw.) ein virtuell simuliertes
Regions-Label. Das ist die Grundlage für die eigentliche, aus der Domäne folgende
Platzierungsentscheidung (P4): CNPG-Replicas und Kafka-Broker werden später bewusst über
verschiedene simulierte Regionen verteilt (Topology-Spread/Anti-Affinity auf einem
Kubernetes-Node-Label `topology.ticketflow/region`), damit ein einzelner Node-/Regionsausfall
nie alle Replicas gleichzeitig trifft – siehe [`../ansible/roles/`](../ansible/roles/) und die
Root-README unter `## Entscheidungen`.

Insgesamt 4 VMs, 3 davon Worker – erfüllt die Vorgabe zu P6 ("mehrere Worker, damit Skalierung und
Verteilung sichtbar werden").

## Struktur

```
terraform/
├── modules/
│   └── compute/   # Keypair, Control-Plane-/Worker-VMs (direkt an DHBWV6, Regions-Metadata)
├── templates/
│   └── hosts.ini.tpl   # Ansible-Inventory-Vorlage
├── main.tf              # Data-Source DHBWV6, Modul-Verdrahtung, Inventory-Generierung
├── variables.tf
├── outputs.tf
├── providers.tf
├── versions.tf
└── terraform.tfvars.example
```

## Verwendung

```bash
cd infra/terraform
cp terraform.tfvars.example terraform.tfvars   # OpenStack-Zugang, Flavor-/Image-Namen eintragen
terraform init
terraform plan
terraform apply
```

Nach `terraform apply` liegt `infra/ansible/inventory/hosts.ini` bereits vor; der nächste Schritt
ist `ansible-playbook -i inventory/hosts.ini playbooks/site.yml` in
[`../ansible/`](../ansible/).

## Offene Punkte

- `image_name`, `control_plane_flavor`, `worker_flavor` sind DHBWCloud-spezifisch und haben
  bewusst **keinen** Default – sie müssen aus dem Portal/der Doku der DHBWCloud übernommen
  werden. `network_name` ist mit `DHBWV6` vorbelegt, `worker_regions` mit drei simulierten
  Regionen.
- Requests/Limits der Flavors werden in `## Entscheidungen` der Root-README begründet, sobald
  konkrete Werte feststehen.
- Da keine eigene Security Group verwaltet wird, gilt die Default-Security-Group des
  DHBWCloud-Projekts. Falls diese SSH/k3s-API/NodePort nicht bereits erlaubt, muss sie einmalig
  passend konfiguriert werden (außerhalb dieses Terraform-Codes, da laut Vorgabe hier explizit
  keine Security-Group-Verwaltung gewünscht ist).
- IPv6-only: `ssh`/`ansible` erreichen die VMs über `access_ip_v6`, das generierte
  `hosts.ini` trägt die IPv6-Adresse direkt als `ansible_host`. k3s muss in
  [`../ansible/`](../ansible/) entsprechend mit `--node-ip`/`--tls-san` auf IPv6 konfiguriert
  werden, und die kubeconfig-Server-URL braucht die Klammer-Syntax `https://[<ipv6>]:6443`
  (siehe `outputs.tf` → `kubeconfig_hint`).
