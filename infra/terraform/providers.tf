# Auth über Application Credentials statt Username/Passwort: die Credential ist bereits auf
# ein Projekt gescoped, daher werden hier bewusst kein tenant_id/user_name/domain angegeben.
provider "openstack" {
  auth_url                      = var.os_auth_url
  region                        = var.os_region
  application_credential_id     = var.os_application_credential_id
  application_credential_secret = var.os_application_credential_secret
}
