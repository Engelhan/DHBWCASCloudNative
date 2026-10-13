# infra/ – Infrastructure as Code (P2)

Zwei Werkzeuge mit klar getrennten Verantwortlichkeiten:

- [`terraform/`](terraform/): erstellt *Ressourcen* auf DHBWCloud (OpenStack) – einen
  SSH-Keypair und VMs im bereits vorhandenen Netzwerk `DHBWV6`. Es werden kein eigenes Netzwerk,
  kein eigener Router, keine eigene Security Group und keine Softwareinstallation innerhalb der
  VMs erzeugt.
- [`ansible/`](ansible/): *konfiguriert* diese VMs – Basis-Hardening, Installation von k3s
  (Server/Agents), Node-Labels und das Taint des Control Plane Nodes für die Platzierungslogik
  (P4, simulierte Regionen), Bootstrap von ArgoCD sowie den gestuften Rollout der
  ApplicationSets. Danach übernimmt [`gitops/`](../gitops/).

Reihenfolge: `terraform apply` → Terraform schreibt das Ansible-Inventory mit den IPs →
`ansible-playbook playbooks/site.yml` → Cluster und ArgoCD sind hoch und die ApplicationSets aus
`gitops/applicationsets/` werden stufenweise angewendet.

Keiner dieser Schritte erfolgt per Hand im DHBWCloud-Portal (Anforderung P2).
