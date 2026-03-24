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

variable "create_service_account_key" {
  description = "Create a service account key (not recommended for production)"
  type        = bool
  default     = false
}

variable "permissions" {
  description = "Additional IAM roles to grant"
  type        = list(string)
  default     = []
}
