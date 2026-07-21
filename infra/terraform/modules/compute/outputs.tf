# DHBWV6 ist ein reines IPv6-Netz (siehe Name) - die VMs erhalten keine IPv4-Adresse, daher
# access_ip_v6 statt access_ip_v4.

output "control_plane" {
  value = {
    name = openstack_compute_instance_v2.control_plane.name
    ip   = openstack_compute_instance_v2.control_plane.access_ip_v6
  }
}

# Ein Eintrag je Worker, inkl. seiner virtuell simulierten Region - Grundlage fuer das
# Ansible-Inventory-Template und spaeter fuer Kubernetes-Node-Labels.
output "workers" {
  value = [
    for inst in openstack_compute_instance_v2.worker : {
      name   = inst.name
      ip     = inst.access_ip_v6
      region = inst.metadata.region
    }
  ]
}
