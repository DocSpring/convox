output "endpoint" {
  value = "https://convox:${random_string.password.result}@api.${var.domain}"
}

# The API password on its own. endpoint embeds the router's domain, which Terraform only knows at apply time
# whenever the router module has changes, so a password parsed from endpoint looks changed in those plans.
output "password" {
  value     = random_string.password.result
  sensitive = true
}
