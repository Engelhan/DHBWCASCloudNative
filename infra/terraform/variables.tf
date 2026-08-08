# --- OpenStack-Zugang (DHBWCloud) ---

variable "os_auth_url" {
  description = "Keystone-Auth-URL der DHBWCloud, z.B. https://newstack.dhbw.cloud:5000/v3"
  type        = string
}

variable "os_region" {
  description = "OpenStack-Region der DHBWCloud"
  type        = string
}

variable "os_application_credential_id" {
  description = "ID des OpenStack Application Credential (Portal: Identity > Application Credentials)"
  type        = string
}

variable "os_application_credential_secret" {
  description = "Secret des OpenStack Application Credential (nur einmal beim Anlegen sichtbar)"
  type        = string
  sensitive   = true
}

# --- Netzwerk ---

variable "network_name" {
  description = "Name des vorhandenen, extern routbaren DHBWCloud-Projektnetzes, an das die VMs direkt gehängt werden (kein eigenes Netz/Subnetz/Router, keine Floating-IPs nötig). Die Default-Security-Group des Projekts gilt, Terraform verwaltet keine eigene."
  type        = string
  default     = "DHBWV6"
}

# --- Compute ---

variable "name_prefix" {
  description = "Präfix für alle erzeugten Ressourcennamen"
  type        = string
  default     = "ticketflow"
}

variable "name_suffix" {
  description = "Optionales Suffix (ohne führenden Bindestrich, z.B. \"carlos\") für alle Ressourcennamen - nützlich, wenn mehrere Studierende auf demselben OpenStack-Projekt eigene, unterscheidbare VMs anlegen"
  type        = string
  default     = ""
}

variable "image_name" {
  description = "Name des Basis-Images in der DHBWCloud-Image-Registry (z.B. eine Ubuntu-LTS-Variante)"
  type        = string
}

variable "control_plane_flavor" {
  description = "Flavor-Name für die Control-Plane-VM"
  type        = string
}

variable "worker_flavor" {
  description = "Flavor-Name für die allgemeinen Worker-VMs"
  type        = string
}

variable "worker_regions" {
  description = "Ein Worker je Listeneintrag, jeweils mit einer virtuell simulierten Region als Metadata-Label. Die DHBWCloud stellt real nur eine Region/AZ bereit; die Verteilung über 'Regionen' wird für Platzierungs-/Verteilungsentscheidungen (P4) nachgebildet, nicht als echtes OpenStack-AZ-Argument."
  type        = list(string)
  default     = ["region-a", "region-b", "region-c"]

  validation {
    condition     = length(var.worker_regions) >= 2
    error_message = "worker_regions muss mindestens 2 Einträge haben, damit mehrere Worker existieren (Vorgabe P4/P6: mehrere Worker, Pod-Verteilung sichtbar)."
  }
}

# --- SSH ---

variable "ssh_public_key_path" {
  description = "Pfad zum öffentlichen SSH-Schlüssel, der in die VMs importiert wird"
  type        = string
  default     = "~/.ssh/ticketflow_dhbwcloud.pub"
}

variable "ssh_private_key_path" {
  description = "Pfad zum privaten SSH-Schlüssel (nur für das generierte Ansible-Inventory, wird von Terraform nicht gelesen)"
  type        = string
  default     = "~/.ssh/ticketflow_dhbwcloud"
}

variable "ssh_user" {
  description = "Login-User des Basis-Images (z.B. ubuntu, debian)"
  type        = string
  default     = "ubuntu"
}
