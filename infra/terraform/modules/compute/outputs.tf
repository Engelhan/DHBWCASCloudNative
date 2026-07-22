# DHBWV6 ist trotz des Namens tatsaechlich dual-stack: die VMs bekommen per DHCP zusaetzlich
# zur IPv6-Adresse eine private, aber tatsaechlich Internet-geroutete IPv4-Adresse
# (10.200.0.0/19, siehe access_ip_v4) auf demselben Interface - urspruenglich angenommen, das
# Netz sei IPv6-only (siehe Git-Historie), auf einem laufenden Cluster widerlegt (VMs erreichen
# github.com per IPv4 problemlos). IPv6 bleibt die primaere Adresse (SSH, kubeconfig-Server-URL,
# k3s --node-ip/--tls-san zuerst) - IPv4 wird zusaetzlich fuer k3s-Dual-Stack-Networking
# durchgereicht, das braucht Pods/Services fuer Egress zu IPv4-only-Zielen wie github.com
# (siehe CLAUDE.md ## Infra layering).

output "control_plane" {
  value = {
    name  = openstack_compute_instance_v2.control_plane.name
    ip    = openstack_compute_instance_v2.control_plane.access_ip_v6
    ip_v4 = openstack_compute_instance_v2.control_plane.access_ip_v4
  }
}

# Ein Eintrag je Worker, inkl. seiner virtuell simulierten Region - Grundlage fuer das
# Ansible-Inventory-Template und spaeter fuer Kubernetes-Node-Labels.
output "workers" {
  value = [
    for inst in openstack_compute_instance_v2.worker : {
      name   = inst.name
      ip     = inst.access_ip_v6
      ip_v4  = inst.access_ip_v4
      region = inst.metadata.region
    }
  ]
}
