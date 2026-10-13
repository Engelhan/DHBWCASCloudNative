variable "name_prefix" {
  type = string
}

variable "name_suffix" {
  description = "Optional suffix (without a leading hyphen, e.g. \"carlos\") for all resource names, so that several students can create distinguishable VMs in the same project"
  type        = string
  default     = ""
}

variable "network_id" {
  type = string
}

variable "keypair_public_key" {
  description = "Contents of the public SSH key"
  type        = string
}

variable "image_name" {
  type = string
}

variable "control_plane_flavor" {
  type = string
}

variable "worker_flavor" {
  type = string
}

variable "worker_regions" {
  description = "One virtually simulated region per worker (DHBWCloud really offers only one region/AZ - the distribution is emulated via a metadata label so that placement/spreading across 'regions' can be demonstrated). One worker per list element."
  type        = list(string)

  validation {
    condition     = length(var.worker_regions) >= 2
    error_message = "worker_regions must have at least 2 entries so that several workers exist (requirement P4)."
  }
}
