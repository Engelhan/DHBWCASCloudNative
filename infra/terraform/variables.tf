# --- OpenStack access (DHBWCloud) ---

variable "os_auth_url" {
  description = "Keystone auth URL of DHBWCloud, e.g. https://newstack.dhbw.cloud:5000/v3"
  type        = string
}

variable "os_region" {
  description = "OpenStack region of DHBWCloud"
  type        = string
}

variable "os_application_credential_id" {
  description = "ID of the OpenStack application credential (portal: Identity > Application Credentials)"
  type        = string
}

variable "os_application_credential_secret" {
  description = "Secret of the OpenStack application credential (only shown once, when created)"
  type        = string
  sensitive   = true
}

# --- Network ---

variable "network_name" {
  description = "Name of the existing, externally routable DHBWCloud project network the VMs are attached to directly (no own network/subnet/router, no floating IPs needed). The project's default security group applies; Terraform does not manage its own."
  type        = string
  default     = "DHBWV6"
}

# --- Compute ---

variable "name_prefix" {
  description = "Prefix for all created resource names. Leave empty: the value then comes from gitops/env/cluster.yaml (ticketflow.namePrefix), the central configuration file of this deployment."
  type        = string
  default     = null
}

variable "name_suffix" {
  description = "Optional suffix (without a leading hyphen, e.g. \"carlos\") for all resource names - useful when several students create their own, distinguishable VMs in the same OpenStack project"
  type        = string
  default     = ""
}

variable "image_name" {
  description = "Name of the base image in the DHBWCloud image registry (e.g. an Ubuntu LTS variant)"
  type        = string
}

variable "control_plane_flavor" {
  description = "Flavor name for the control-plane VM"
  type        = string
}

variable "worker_flavor" {
  description = "Flavor name for the general-purpose worker VMs"
  type        = string
}

variable "worker_regions" {
  description = "One worker per list entry, each with a virtually simulated region as metadata label. DHBWCloud really provides only one region/AZ; the distribution across 'regions' is emulated for placement/spreading decisions (P4), not passed as a real OpenStack AZ argument."
  type        = list(string)
  default     = ["region-a", "region-b", "region-c"]

  validation {
    condition     = length(var.worker_regions) >= 2
    error_message = "worker_regions must have at least 2 entries so that several workers exist (requirement P4: several workers, visible pod distribution)."
  }
}

# --- SSH ---

variable "ssh_public_key_path" {
  description = "Path to the public SSH key that is imported into the VMs"
  type        = string
  default     = "~/.ssh/ticketflow_dhbwcloud.pub"
}

variable "ssh_private_key_path" {
  description = "Path to the private SSH key (only written into the generated Ansible inventory, never read by Terraform)"
  type        = string
  default     = "~/.ssh/ticketflow_dhbwcloud"
}

variable "ssh_user" {
  description = "Login user of the base image (e.g. ubuntu, debian)"
  type        = string
  default     = "ubuntu"
}
