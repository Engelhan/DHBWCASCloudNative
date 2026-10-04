# infra/ – Infrastructure as Code (P2)

Two tools with clearly separated responsibilities:

- [`terraform/`](terraform/): creates *resources* on DHBWCloud (OpenStack) – a keypair and VMs on
  the existing `DHBWV6` network. No own network/router, no own security group, no software setup
  inside the VMs.
- [`ansible/`](ansible/): *configures* those VMs – base hardening, k3s installation
  (server/agents), node labels and the control-plane taint for the placement decision (P4,
  simulated regions), ArgoCD bootstrap, and the staged rollout of the ApplicationSets. From there
  on, [`gitops/`](../gitops/) takes over.

Order: `terraform apply` → Terraform writes the Ansible inventory (IPs) →
`ansible-playbook playbooks/site.yml` → cluster + ArgoCD are up, the ApplicationSets from
`gitops/applicationset.yaml` are rolled out stage by stage.

None of these steps is a manual click in the DHBWCloud portal (requirement P2).
