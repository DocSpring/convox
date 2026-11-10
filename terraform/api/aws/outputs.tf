output "endpoint" {
  value = module.k8s.endpoint
}

output "password" {
  value     = module.k8s.password
  sensitive = true
}
