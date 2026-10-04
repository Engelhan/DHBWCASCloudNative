output "control_plane_ip" {
  description = "IPv6 address of the control-plane VM on DHBWV6 (no floating IP needed, SSH/k3s API reachable directly; primary address, see control_plane_ip_v4 for the additional IPv4 address)"
  value       = module.compute.control_plane.ip
}

output "control_plane_ip_v4" {
  description = "Additional DHCP-assigned IPv4 address of the control-plane VM (despite its name, DHBWV6 is dual-stack) - used for k3s dual-stack networking, not for SSH/kubeconfig"
  value       = module.compute.control_plane.ip_v4
}

output "worker_ips" {
  description = "IPv6 addresses of all worker VMs, keyed by name"
  value       = { for w in module.compute.workers : w.name => w.ip }
}

output "worker_ips_v4" {
  description = "Additional IPv4 addresses of all worker VMs, keyed by name"
  value       = { for w in module.compute.workers : w.name => w.ip_v4 }
}

output "kubeconfig_hint" {
  description = "How to fetch the kubeconfig from the control plane (Ansible does this automatically; this is the manual fallback). IPv6 addresses need square brackets in the server URL."
  value       = "ssh ${var.ssh_user}@${module.compute.control_plane.ip} sudo cat /etc/rancher/k3s/k3s.yaml | sed 's#https://127.0.0.1:6443#https://[${module.compute.control_plane.ip}]:6443#' > kubeconfig"
}
