# Jedes Modul, das einen Provider direkt nutzt, muss dessen Quelladresse selbst deklarieren -
# das wird nicht automatisch vom Root-Modul vererbt. Ohne diesen Block würde Terraform für den
# lokalen Namen "openstack" den Default-Namespace hashicorp/openstack annehmen (falsch; wir
# wollen terraform-provider-openstack/openstack). Die Versionsvorgabe selbst bleibt bewusst nur
# im Root (versions.tf), damit es nur eine Stelle zum Pflegen gibt.
terraform {
  required_providers {
    openstack = {
      source = "terraform-provider-openstack/openstack"
    }
  }
}
