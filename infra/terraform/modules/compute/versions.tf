# Every module that uses a provider directly must declare its source address itself - it is not
# inherited from the root module. Without this block Terraform would assume the default namespace
# hashicorp/openstack for the local name "openstack" (wrong; we want
# terraform-provider-openstack/openstack). The version constraint itself deliberately lives only
# in the root (versions.tf), so there is a single place to maintain it.
terraform {
  required_providers {
    openstack = {
      source = "terraform-provider-openstack/openstack"
    }
  }
}
