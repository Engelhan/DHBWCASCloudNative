# infra/ – Infrastructure as Code (P2)

Zwei Werkzeuge, klar getrennte Verantwortung:

- [`terraform/`](terraform/): erzeugt *Ressourcen* auf der DHBWCloud (OpenStack) – Keypair und
  VMs am vorhandenen Netz `DHBWV6`. Kein eigenes Netz/Router, keine eigene Security Group, kein
  Software-Setup innerhalb der VMs.
- [`ansible/`](ansible/): *konfiguriert* diese VMs – Basis-Härtung, k3s-Installation
  (Server/Agents), Node-Labels für die Platzierungsentscheidung (P4, simulierte Regionen),
  Bootstrap von ArgoCD. Ab dem ArgoCD-Bootstrap übernimmt [`gitops/`](../gitops/) den Rest.

Reihenfolge: `terraform apply` → Terraform-Output (IPs) füttert das Ansible-Inventory →
`ansible-playbook site.yml` → Cluster + ArgoCD stehen → `kubectl apply -f
gitops/apps/app-of-apps.yaml`.

Kein Schritt davon ist ein manueller Klick im DHBWCloud-Portal (Vorgabe P2).
