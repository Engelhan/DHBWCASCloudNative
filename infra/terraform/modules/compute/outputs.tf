# Despite its name, DHBWV6 is actually dual-stack: in addition to the IPv6 address, the VMs get a
# private but genuinely internet-routed IPv4 address via DHCP (10.200.0.0/19, see access_ip_v4) on
# the same interface - originally assumed to be IPv6-only (see git history), disproved on a running
# cluster (the VMs reach github.com over IPv4 without issues). IPv6 stays the primary address (SSH,
# kubeconfig server URL, k3s --node-ip/--tls-san first) - IPv4 is passed through additionally for
# k3s dual-stack networking, which pods/services need for egress to IPv4-only targets such as
# github.com (see CLAUDE.md ## Infra layering).

output "control_plane" {
  value = {
    name  = openstack_compute_instance_v2.control_plane.name
    ip    = openstack_compute_instance_v2.control_plane.access_ip_v6
    ip_v4 = openstack_compute_instance_v2.control_plane.access_ip_v4
  }
}

# One entry per worker, including its virtually simulated region - the basis for the Ansible
# inventory template and, later, for the Kubernetes node labels.
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
