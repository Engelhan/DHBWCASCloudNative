# VMs for the k3s cluster: 1 control plane + one worker per entry in var.worker_regions.
#
# All VMs are attached directly to the existing, externally routable DHBWV6 network (see root
# main.tf) - no own floating IPs, no self-managed security group (the project default applies).
# Despite its name, DHBWV6 is dual-stack: the instances get both an IPv6 and a private but
# internet-routed IPv4 address via DHCP (see outputs.tf). IPv6 stays primary (--node-ip,
# --tls-san, kubeconfig server URL in bracket syntax), IPv4 is passed through additionally for k3s
# dual-stack networking.
#
# Placement (P4): DHBWCloud really provides only one region/AZ. To still demonstrate a
# region-aware distribution of CNPG replicas and Kafka brokers across the workers, each worker is
# assigned a virtually simulated region name via metadata (no real OpenStack availability_zone
# argument, since these regions do not physically exist). On the Kubernetes side this later
# becomes the node label topology.ticketflow/region=<region>, which topology spread constraints /
# anti-affinity schedule against - see infra/ansible/roles/node_labels.

locals {
  # Optional name suffix (e.g. your first name) so that several students can create their own,
  # clearly distinguishable VMs in parallel in the same OpenStack project without changing the
  # existing name_prefix (see var.name_prefix). Default empty -> no behavioral difference for
  # anyone who does not set the variable.
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
