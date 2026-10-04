# Auth via application credentials instead of username/password: the credential is already scoped
# to a project, so tenant_id/user_name/domain are deliberately not set here.
provider "openstack" {
  auth_url                      = var.os_auth_url
  region                        = var.os_region
  application_credential_id     = var.os_application_credential_id
  application_credential_secret = var.os_application_credential_secret
}
