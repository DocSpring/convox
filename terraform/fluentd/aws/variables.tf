variable "access_log_retention_in_days" {
  default = "7"
  type    = string
}

variable "cluster" {
  type = string
}

variable "app_cloudwatch_disable" {
  type    = bool
  default = false
}

variable "fluentd_disable" {
  type    = bool
  default = false
}

variable "fluentd_memory" {
  type    = string
  default = "200Mi"
}

// for eks addons dependency
variable "eks_addons" {}

variable "namespace" {
  type = string
}

variable "rack" {
  type = string
}

variable "oidc_arn" {
  type = string
}

variable "oidc_sub" {
  type = string
}

variable "syslog" {
  default = ""

  # DocSpring fork: the fluentd image in main.tf (fluent/fluentd-kubernetes-daemonset)
  # has no `syslog` output plugin. That plugin only exists in convox/fluentd. With an
  # endpoint set, target.conf uses `@type syslog`, fluentd fails to start and the
  # rollout crash-loops while `convox rack update` still reports success.
  validation {
    condition     = var.syslog == ""
    error_message = "The syslog rack param isn't supported with this fork's fluentd image (no syslog output plugin). Use convox/fluentd 1.19+ with the syslog plugin before setting it."
  }
}
