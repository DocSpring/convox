output "api" {
  value = module.rack.api
}

# The rack API password, known at plan time even when the router's domain (part of api) is not.
output "api_password" {
  value     = module.rack.api_password
  sensitive = true
}

output "cluster" {
  value = module.cluster
}

output "endpoint" {
  value = module.rack.endpoint
}

output "endpoint_internal" {
  value = module.rack.endpoint_internal
}

output "release" {
  value = local.release
}
