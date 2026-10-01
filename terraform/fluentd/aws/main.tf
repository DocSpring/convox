data "aws_caller_identity" "current" {}
data "aws_region" "current" {}

locals {
  tags = {
    System  = "convox"
    Cluster = var.cluster
  }
}

module "k8s" {
  source = "../k8s"

  providers = {
    kubernetes = kubernetes
  }

  fluentd_disable = var.fluentd_disable
  fluentd_memory  = var.fluentd_memory

  cluster = var.cluster
  # convox/fluentd:1.13-all runs fluentd 1.7.4 with kubernetes_metadata_filter
  # 2.3.0, which reads the service account token once at startup. EKS 1.34+
  # expires that token after 24 hours, after which every pod lookup gets a 401,
  # records lose their labels, and the rewrite_tag_filter rules drop every app
  # log line without an error. kubernetes_metadata_filter 2.11+ recreates its
  # client (re-reading the token) on 401. This image has fluentd 1.19.3,
  # kubernetes_metadata_filter 3.8, fluent-plugin-cloudwatch-logs 0.15 and the
  # other plugins the config uses (multi-format-parser, parser-cri,
  # rewrite-tag-filter, record-modifier). Pinned by digest (amd64 + arm64).
  image     = "fluent/fluentd-kubernetes-daemonset:v1.19.3-debian-cloudwatch-1.1@sha256:268c29d7908ce779e200cf08d26e3ab52c0eb965fdae51e8d424730c0e311166"
  namespace = var.namespace
  rack      = var.rack

  target = templatefile("${path.module}/target.conf.tpl", {
    access_log_retention   = var.access_log_retention_in_days,
    app_cloudwatch_disable = var.app_cloudwatch_disable,
    rack                   = var.rack,
    region                 = data.aws_region.current.name,
    syslog                 = compact(split(",", var.syslog))
  })

  annotations = {
    "eks.amazonaws.com/role-arn" = aws_iam_role.fluentd.arn,
    "iam.amazonaws.com/role"     = aws_iam_role.fluentd.arn,
    "convox.com/dummy"           = var.access_log_retention_in_days,
  }

  env = {
    AWS_REGION = data.aws_region.current.name
  }
}
