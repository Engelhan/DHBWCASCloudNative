variable "name_prefix" {
  type = string
}

variable "network_id" {
  type = string
}

variable "keypair_public_key" {
  description = "Inhalt des oeffentlichen SSH-Schluessels"
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
  description = "Eine virtuell simulierte Region je Worker (DHBWCloud bietet real nur eine Region/AZ - die Verteilung wird per Metadata-Label nachgebildet, damit Platzierung/Verteilung ueber 'Regionen' hinweg gezeigt werden kann). Ein Worker pro Listenelement."
  type        = list(string)

  validation {
    condition     = length(var.worker_regions) >= 2
    error_message = "worker_regions muss mindestens 2 Eintraege haben, damit mehrere Worker existieren (Vorgabe P4/P6)."
  }
}
