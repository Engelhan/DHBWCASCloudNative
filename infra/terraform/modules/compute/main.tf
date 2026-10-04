# 1 control plane + one worker per worker_regions entry, all on DHBWV6.
# P4: DHBWCloud has one AZ, so each worker gets a simulated region as metadata (later the node
# label topology.ticketflow/region).

locals {
  # Optional suffix so several students can share a project.
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
