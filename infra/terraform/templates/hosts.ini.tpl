; Automatisch von Terraform generiert (resource "local_file" "ansible_inventory" in main.tf).
; Nicht manuell bearbeiten - Aenderungen gehen beim naechsten "terraform apply" verloren.

[control_plane]
${control_plane_name} ansible_host=${control_plane_ip}

[workers]
%{ for w in workers ~}
${w.name} ansible_host=${w.ip} region=${w.region}
%{ endfor ~}

[k3s_cluster:children]
control_plane
workers

[k3s_cluster:vars]
ansible_user=${ssh_user}
ansible_ssh_private_key_file=${ssh_private_key_path}
