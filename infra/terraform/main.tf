# Central configuration - the same file ArgoCD, all Helm charts and Ansible read their
# deployment-specific values from (see gitops/env/cluster.yaml). Terraform only needs the name
# prefix from it; everything else there is Kubernetes/DNS level. Credentials are deliberately NOT
# in it, they stay in the gitignored terraform.tfvars.
locals {
  ticketflow = yamldecode(file("${path.module}/../../gitops/env/cluster.yaml")).ticketflow
}

# No own network/subnet/router and no self-managed security group: DHBWCloud already provides
# DHBWV6, an externally routable project network including a default security group. It is only
# looked up, never created.
data "openstack_networking_network_v2" "existing" {
  name = var.network_name
}

module "compute" {
  source = "./modules/compute"

  # var.name_prefix overrides the central value if set (default: unset).
  name_prefix          = coalesce(var.name_prefix, local.ticketflow.namePrefix)
  name_suffix          = var.name_suffix
  network_id           = data.openstack_networking_network_v2.existing.id
  keypair_public_key   = file(pathexpand(var.ssh_public_key_path))
  image_name           = var.image_name
  control_plane_flavor = var.control_plane_flavor
  worker_flavor        = var.worker_flavor
  worker_regions       = var.worker_regions
}

# Generates the Ansible inventory directly from the Terraform result so that "one run brings
# everything up" (P2) - no manual copying of IPs between Terraform and Ansible.
resource "local_file" "ansible_inventory" {
  filename = "${path.module}/../ansible/inventory/hosts.ini"

  content = templatefile("${path.module}/templates/hosts.ini.tpl", {
    control_plane_name   = module.compute.control_plane.name
    control_plane_ip     = module.compute.control_plane.ip
    control_plane_ip_v4  = module.compute.control_plane.ip_v4
    workers              = module.compute.workers
    ssh_user             = var.ssh_user
    ssh_private_key_path = var.ssh_private_key_path
  })
}
