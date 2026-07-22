output "control_plane_ip" {
  description = "IPv6-Adresse der Control-Plane-VM am Netz DHBWV6 (kein Floating IP nötig, SSH/k3s-API direkt erreichbar; primäre Adresse, siehe control_plane_ip_v4 für die zusätzliche IPv4-Adresse)"
  value       = module.compute.control_plane.ip
}

output "control_plane_ip_v4" {
  description = "Zusätzliche, per DHCP zugewiesene IPv4-Adresse der Control-Plane-VM (DHBWV6 ist entgegen dem Namen dual-stack) — genutzt für k3s-Dual-Stack-Networking, nicht für SSH/kubeconfig"
  value       = module.compute.control_plane.ip_v4
}

output "worker_ips" {
  description = "IPv6-Adressen aller Worker-VMs, keyed by Name"
  value       = { for w in module.compute.workers : w.name => w.ip }
}

output "worker_ips_v4" {
  description = "Zusätzliche IPv4-Adressen aller Worker-VMs, keyed by Name"
  value       = { for w in module.compute.workers : w.name => w.ip_v4 }
}

output "kubeconfig_hint" {
  description = "Wie man die kubeconfig von der Control-Plane holt (Ansible erledigt das automatisch, dies ist der manuelle Fallback). IPv6-Adressen brauchen im Server-URL eckige Klammern."
  value       = "ssh ${var.ssh_user}@${module.compute.control_plane.ip} sudo cat /etc/rancher/k3s/k3s.yaml | sed 's#https://127.0.0.1:6443#https://[${module.compute.control_plane.ip}]:6443#' > kubeconfig"
}
