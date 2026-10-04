# Modules must declare the provider source themselves; the version is pinned in the root.
terraform {
  required_providers {
    openstack = {
      source = "terraform-provider-openstack/openstack"
    }
  }
}
