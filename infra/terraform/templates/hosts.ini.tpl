; Generated automatically by Terraform (resource "local_file" "ansible_inventory" in main.tf).
; Do not edit by hand - changes are lost on the next "terraform apply".

[control_plane]
${control_plane_name} ansible_host=${control_plane_ip} node_ip_v4=${control_plane_ip_v4}

[workers]
%{ for w in workers ~}
${w.name} ansible_host=${w.ip} node_ip_v4=${w.ip_v4} region=${w.region}
%{ endfor ~}

[k3s_cluster:children]
control_plane
workers

[k3s_cluster:vars]
ansible_user=${ssh_user}
ansible_ssh_private_key_file=${ssh_private_key_path}
