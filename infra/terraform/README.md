# infra/terraform

Provisions nothing but the resource layer on DHBWCloud via the **OpenStack provider**
(`terraform-provider-openstack/openstack`). No Kubernetes and no app configuration here – that is
handled by [`../ansible/`](../ansible/). There is deliberately **one** environment: the lab has no
productive parallel operation, so a second Terraform workspace/directory tree would just be
unjustified extra effort. (The `values-dev.yaml`/`values-prod.yaml` required by W1 refer to the
Helm charts, not to the infrastructure.)

## Resources

- No own network/subnet/router and no self-managed security group: DHBWCloud already provides
  **DHBWV6**, an externally routable project network including a default security group.
  `main.tf` only looks this network up via a data source and creates nothing.
- One SSH keypair (imported public key)
- VMs ([`modules/compute`](modules/compute/)), each attached directly to DHBWV6 with a directly
  reachable `access_ip_v6` (primary address for SSH/kubeconfig) **and** a DHCP-assigned, genuinely
  internet-routed `access_ip_v4` (**despite its name, DHBWV6 is dual-stack**, not IPv6-only —
  Terraform only passes `access_ip_v4` through additionally so that k3s can run dual-stack, see
  [`../ansible/README.md`](../ansible/README.md) ## IPv6 (DHBWV6) and dual-stack):
  - 1× `control-plane` (k3s server)
  - 1× worker per entry in `var.worker_regions` (default 3: `region-a`, `region-b`, `region-c`),
    all with the same flavor
- A generated `infra/ansible/inventory/hosts.ini` (`local_file` resource in `main.tf`, from
  [`templates/hosts.ini.tpl`](templates/hosts.ini.tpl)) – automates exactly the step
  "Terraform output → Ansible inventory" and makes it part of `terraform apply` instead of copying
  IPs by hand.

## Why "regions" instead of a storage-optimized worker

DHBWCloud really provides only one region/AZ; a flavor difference such as "more/faster storage"
would only have been an artificial placement rule taken over from the lecture. Instead, every
worker gets a virtually simulated region label via metadata (`region = "region-a"` etc.). This is
the basis for the actual placement decision that follows from the domain (P4): CNPG replicas,
Kafka brokers and every service running twice are deliberately spread across different simulated
regions (topology spread / anti-affinity on the Kubernetes node label
`topology.ticketflow/region`), so that a single node/region failure never hits all replicas at
once – see [`../ansible/roles/`](../ansible/roles/) and the root README under `## Entscheidungen`.

4 VMs in total, 3 of them workers – fulfills the P6 requirement ("several workers so that scaling
and distribution become visible").

## Structure

```
terraform/
├── modules/
│   └── compute/   # keypair, control-plane/worker VMs (directly on DHBWV6, region metadata)
├── templates/
│   └── hosts.ini.tpl   # Ansible inventory template
├── main.tf              # DHBWV6 data source, module wiring, inventory generation
├── variables.tf
├── outputs.tf
├── providers.tf
├── versions.tf
└── terraform.tfvars.example
```

## Usage

```bash
cd infra/terraform
cp terraform.tfvars.example terraform.tfvars   # fill in OpenStack access, flavor/image names
terraform init
terraform plan
terraform apply
```

After `terraform apply`, `infra/ansible/inventory/hosts.ini` already exists; the next step is
`ansible-playbook -i inventory/hosts.ini playbooks/site.yml` in [`../ansible/`](../ansible/).

## Open points

- `image_name`, `control_plane_flavor`, `worker_flavor` are DHBWCloud-specific and deliberately
  have **no** default – they have to be taken from the DHBWCloud portal/documentation.
  `network_name` defaults to `DHBWV6`, `worker_regions` to three simulated regions.
- Requests/limits of the flavors are justified in `## Entscheidungen` of the root README once
  concrete values are fixed.
- Since no own security group is managed, the default security group of the DHBWCloud project
  applies. If it does not already allow SSH/k3s API/NodePort, it has to be configured accordingly
  once (outside this Terraform code, since the requirements explicitly ask for no security-group
  management here).
- IPv6-primary: `ssh`/`ansible` reach the VMs via `access_ip_v6`; the generated `hosts.ini` carries
  the IPv6 address as `ansible_host` and additionally `access_ip_v4` as `node_ip_v4` (its own host
  var, see `hosts.ini.tpl`). k3s runs dual-stack in [`../ansible/`](../ansible/)
  (`--node-ip`/`--cluster-cidr`/`--service-cidr` with IPv6 first, IPv4 additionally — see the
  README there, ## IPv6 (DHBWV6) and dual-stack), the kubeconfig server URL stays IPv6-only with
  bracket syntax `https://[<ipv6>]:6443` (see `outputs.tf` → `kubeconfig_hint`).
