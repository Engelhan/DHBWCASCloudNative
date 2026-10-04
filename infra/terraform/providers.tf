# Application credential (already project-scoped).
provider "openstack" {
  auth_url                      = var.os_auth_url
  region                        = var.os_region
  application_credential_id     = var.os_application_credential_id
  application_credential_secret = var.os_application_credential_secret
}
