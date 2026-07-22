# Kein eigenes Netz/Subnetz/Router und keine selbst verwaltete Security Group: die DHBWCloud
# stellt mit DHBWV6 bereits ein extern routbares Projektnetz samt Default-Security-Group
# bereit. Es wird nur nachgeschaut, nicht angelegt.
data "openstack_networking_network_v2" "existing" {
  name = var.network_name
}

module "compute" {
  source = "./modules/compute"

  name_prefix           = var.name_prefix
  network_id            = data.openstack_networking_network_v2.existing.id
  keypair_public_key    = file(pathexpand(var.ssh_public_key_path))
  image_name            = var.image_name
  control_plane_flavor  = var.control_plane_flavor
  worker_flavor         = var.worker_flavor
  worker_regions        = var.worker_regions
}

# Generiert das Ansible-Inventory direkt aus dem Terraform-Ergebnis, damit "ein Ablauf alles
# hochbringt" (P2) - kein manuelles Abtippen von IPs zwischen Terraform und Ansible.
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
