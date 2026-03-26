variable "cluster_name" {
  description = "GKE cluster name"
  type        = string
}

variable "app_name" {
  description = "Application name"
  type        = string
}

variable "environment" {
  description = "Environment name"
  type        = string
}

variable "gcp_project" {
  description = "GCP project ID"
  type        = string
}

variable "namespace" {
  description = "Kubernetes namespace"
  type        = string
  default     = "default"
}

variable "ksa_name" {
  description = "Kubernetes service account name"
  type        = string
  default     = "app-workload"
}

variable "enable_network_policies" {
  description = "Enable Kubernetes NetworkPolicies"
  type        = bool
  default     = true
}

variable "enable_pod_security_policy" {
  description = "Enable Pod Security Policy"
  type        = bool
  default     = true
}

variable "manage_kubernetes_security_resources" {
  description = "Create Kubernetes RBAC/NetworkPolicy/PSP resources from this module"
  type        = bool
  default     = true
}

variable "create_service_account_key" {
  description = "Create a service account key (not recommended for production)"
  type        = bool
  default     = false
  validation {
    condition     = !(var.create_service_account_key && var.environment == "prod")
    error_message = "Service account keys are blocked in production. Use Workload Identity instead."
  }
}

variable "permissions" {
  description = "Additional IAM roles to grant"
  type        = list(string)
  default     = []
}

variable "pubsub_role" {
  description = "Pub/Sub IAM role granted to app workload"
  type        = string
  default     = "roles/pubsub.publisher"
  validation {
    condition = contains([
      "roles/pubsub.publisher",
      "roles/pubsub.subscriber",
      "roles/pubsub.viewer",
      "roles/pubsub.editor",
    ], var.pubsub_role)
    error_message = "pubsub_role must be one of: roles/pubsub.publisher, roles/pubsub.subscriber, roles/pubsub.viewer, roles/pubsub.editor."
  }
}

variable "https_egress_cidrs" {
  description = "Allowed HTTPS egress CIDRs for workloads"
  type        = list(string)
  default = [
    "199.36.153.8/30",
    "10.0.0.0/8",
    "172.16.0.0/12",
    "192.168.0.0/16",
  ]
}

variable "db_egress_cidrs" {
  description = "Allowed PostgreSQL egress CIDRs for workloads"
  type        = list(string)
  default = [
    "10.0.0.0/8",
    "172.16.0.0/12",
    "192.168.0.0/16",
  ]
}
