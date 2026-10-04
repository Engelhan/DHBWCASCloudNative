# DHBWV6 is dual-stack: IPv6 primary, IPv4 passed on for k3s dual-stack.

output "control_plane" {
  value = {
    name  = openstack_compute_instance_v2.control_plane.name
    ip    = openstack_compute_instance_v2.control_plane.access_ip_v6
    ip_v4 = openstack_compute_instance_v2.control_plane.access_ip_v4
  }
}

# One entry per worker incl. its simulated region.
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
