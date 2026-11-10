output "api" {
  value = module.api.endpoint
}

output "api_password" {
  value     = module.api.password
  sensitive = true
}

output "endpoint" {
  value = module.router.endpoint
}

output "endpoint_internal" {
  value = module.router.endpoint_internal
}
