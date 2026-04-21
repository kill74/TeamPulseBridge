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
  validation {
    condition = alltrue([
      for role in var.permissions :
      !contains([
        "roles/owner",
        "roles/editor",
        "roles/resourcemanager.projectIamAdmin",
        "roles/iam.roleAdmin",
        "roles/iam.securityAdmin",
        "roles/iam.serviceAccountAdmin",
        "roles/iam.serviceAccountKeyAdmin",
        "roles/iam.serviceAccountUser",
        "roles/iam.serviceAccountOpenIdTokenCreator",
        "roles/iam.serviceAccountTokenCreator",
        "roles/iam.workloadIdentityPoolAdmin",
        "roles/orgpolicy.policyAdmin",
      ], role)
    ])
    error_message = "permissions contains a deny-listed role. Remove high-privilege roles (owner/editor/IAM admin variants)."
  }
  validation {
    condition = var.environment != "prod" || length(var.permissions) == 0 || (
      var.allow_production_iam_exceptions &&
      length(trimspace(var.production_iam_exception_justification)) >= 20 &&
      length(regexall("[A-Z]{2,10}-[0-9]{1,6}", var.production_iam_exception_justification)) > 0 &&
      length(regexall("20[0-9]{2}-[01][0-9]-[0-3][0-9]", var.production_iam_exception_justification)) > 0
    )
    error_message = "In production, additional IAM roles require allow_production_iam_exceptions=true and a justification with at least 20 characters including a ticket (e.g. SEC-1234) and expiry date (YYYY-MM-DD)."
  }
}

variable "allow_production_iam_exceptions" {
  description = "Allow additional IAM role grants in production when a documented exception is required"
  type        = bool
  default     = false
}

variable "production_iam_exception_justification" {
  description = "Required justification for production IAM exceptions (ticket, risk acceptance, and expiry)"
  type        = string
  default     = ""
  validation {
    condition = var.environment != "prod" || !var.allow_production_iam_exceptions || (
      length(trimspace(var.production_iam_exception_justification)) >= 20 &&
      length(regexall("[A-Z]{2,10}-[0-9]{1,6}", var.production_iam_exception_justification)) > 0 &&
      length(regexall("20[0-9]{2}-[01][0-9]-[0-3][0-9]", var.production_iam_exception_justification)) > 0
    )
    error_message = "production_iam_exception_justification must include at least 20 characters, a ticket (e.g. SEC-1234), and an expiry date (YYYY-MM-DD) when production IAM exceptions are enabled."
  }
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
    ], var.pubsub_role)
    error_message = "pubsub_role must be one of: roles/pubsub.publisher, roles/pubsub.subscriber, roles/pubsub.viewer."
  }
  validation {
    condition = var.environment != "prod" || var.pubsub_role == "roles/pubsub.publisher" || (
      var.allow_production_iam_exceptions &&
      length(trimspace(var.production_iam_exception_justification)) >= 20 &&
      length(regexall("[A-Z]{2,10}-[0-9]{1,6}", var.production_iam_exception_justification)) > 0 &&
      length(regexall("20[0-9]{2}-[01][0-9]-[0-3][0-9]", var.production_iam_exception_justification)) > 0
    )
    error_message = "In production, pubsub_role must remain roles/pubsub.publisher unless a documented exception is enabled with a ticket and expiry date."
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
