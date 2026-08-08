# VMs fuer den k3s-Cluster: 1 Control-Plane + ein Worker je Eintrag in var.worker_regions.
#
# Alle VMs haengen direkt am vorhandenen, extern routbaren Netz DHBWV6 (siehe root main.tf) -
# keine eigenen Floating-IPs, keine selbst verwaltete Security Group (Projekt-Default gilt).
# DHBWV6 ist trotz des Namens dual-stack: die Instanzen bekommen per DHCP sowohl eine
# IPv6- als auch eine private, aber Internet-geroutete IPv4-Adresse (siehe outputs.tf). IPv6
# bleibt primaer (--node-ip, --tls-san, kubeconfig-Server-URL in Klammer-Syntax), IPv4 wird
# zusaetzlich fuer k3s-Dual-Stack-Networking durchgereicht.
#
# Platzierung (P4): die DHBWCloud stellt real nur eine Region/AZ bereit. Um trotzdem eine
# region-bewusste Verteilung von CNPG-Replicas und Kafka-Brokern ueber die Worker zeigen zu
# koennen, wird jedem Worker per Metadata ein virtuell simulierter Regionsname zugewiesen
# (kein echtes OpenStack availability_zone-Argument, da diese Regionen physisch nicht
# existieren). Kubernetes-seitig wird daraus spaeter ein Node-Label
# topology.ticketflow/region=<region>, gegen das Topology-Spread-Constraints/Anti-Affinity
# fuer CNPG und Kafka planen - siehe infra/ansible/roles/node_labels.

locals {
  # Optionales Namens-Suffix (z.B. eigener Vorname), damit mehrere Studierende parallel auf
  # demselben OpenStack-Projekt eigene, klar unterscheidbare VMs anlegen koennen, ohne den
  # bestehenden name_prefix (Praefix, siehe var.name_prefix) zu veraendern. Default leer -> kein
  # Verhaltensunterschied fuer alle, die die Variable nicht setzen.
  name_suffix = var.name_suffix != "" ? "-${var.name_suffix}" : ""
}

resource "openstack_compute_keypair_v2" "this" {
  name       = "${var.name_prefix}-key${local.name_suffix}"
  public_key = var.keypair_public_key
}

resource "openstack_compute_instance_v2" "control_plane" {
  name        = "${var.name_prefix}-control-plane${local.name_suffix}"
  image_name  = var.image_name
  flavor_name = var.control_plane_flavor
  key_pair    = openstack_compute_keypair_v2.this.name

  network {
    uuid = var.network_id
  }

  metadata = {
    role = "control-plane"
  }
}

resource "openstack_compute_instance_v2" "worker" {
  count       = length(var.worker_regions)
  name        = "${var.name_prefix}-worker-${count.index + 1}${local.name_suffix}"
  image_name  = var.image_name
  flavor_name = var.worker_flavor
  key_pair    = openstack_compute_keypair_v2.this.name

  network {
    uuid = var.network_id
  }

  metadata = {
    role   = "worker"
    region = var.worker_regions[count.index]
  }
}
