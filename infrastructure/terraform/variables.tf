# GCP Configuration
variable "gcp_project" {
  description = "GCP project ID"
  type        = string
}

variable "region" {
  description = "GCP region"
  type        = string
  default     = "us-central1"
}

variable "enable_multi_region" {
  description = "Enable active-active multi-region topology"
  type        = bool
  default     = false
}

variable "secondary_region" {
  description = "Secondary GCP region for active-active topology"
  type        = string
  default     = ""
}

# Project Configuration
variable "project_name" {
  description = "Project name"
  type        = string
  default     = "teampulse"
}

variable "environment" {
  description = "Environment (staging, prod)"
  type        = string
  validation {
    condition     = contains(["staging", "prod"], var.environment)
    error_message = "Environment must be either 'staging' or 'prod'."
  }
}

variable "app_name" {
  description = "Application name"
  type        = string
  default     = "ingestion-gateway"
}

# Network Configuration
variable "gke_subnet_cidr" {
  description = "CIDR for GKE subnet"
  type        = string
  default     = "10.0.0.0/24"
}

variable "secondary_gke_subnet_cidr" {
  description = "CIDR for secondary region GKE subnet"
  type        = string
  default     = "10.10.0.0/24"
}

variable "pods_cidr" {
  description = "CIDR for pods secondary range"
  type        = string
  default     = "10.1.0.0/16"
}

variable "secondary_pods_cidr" {
  description = "CIDR for secondary region pods secondary range"
  type        = string
  default     = "10.11.0.0/16"
}

variable "services_cidr" {
  description = "CIDR for services secondary range"
  type        = string
  default     = "10.2.0.0/16"
}

variable "secondary_services_cidr" {
  description = "CIDR for secondary region services secondary range"
  type        = string
  default     = "10.12.0.0/16"
}

variable "db_subnet_cidr" {
  description = "CIDR for database subnet"
  type        = string
  default     = "10.3.0.0/24"
}

variable "secondary_db_subnet_cidr" {
  description = "CIDR for secondary region database subnet"
  type        = string
  default     = "10.13.0.0/24"
}

variable "enable_ssh_access" {
  description = "Enable SSH access to cluster nodes"
  type        = bool
  default     = false
}

variable "ssh_source_ranges" {
  description = "Source CIDR ranges for SSH access"
  type        = list(string)
  default     = []
}

# GKE Cluster Configuration
variable "gke_initial_node_count" {
  description = "Initial number of nodes in default pool"
  type        = number
  default     = 1
}

variable "gke_min_node_count" {
  description = "Minimum nodes in default pool"
  type        = number
  default     = 1
}

variable "gke_max_node_count" {
  description = "Maximum nodes in default pool"
  type        = number
  default     = 5
}

variable "gke_machine_type" {
  description = "Machine type for default pool"
  type        = string
  default     = "n1-standard-2"
}

variable "gke_disk_size_gb" {
  description = "Node disk size in GB"
  type        = number
  default     = 100
}

variable "gke_preemptible_nodes" {
  description = "Use preemptible nodes in default pool"
  type        = bool
  default     = true
}

variable "cluster_min_cpu" {
  description = "Cluster minimum CPU limit"
  type        = number
  default     = 2
}

variable "cluster_max_cpu" {
  description = "Cluster maximum CPU limit"
  type        = number
  default     = 64
}

variable "cluster_min_memory" {
  description = "Cluster minimum memory limit (GB)"
  type        = number
  default     = 8
}

variable "cluster_max_memory" {
  description = "Cluster maximum memory limit (GB)"
  type        = number
  default     = 256
}

# Workload Node Pool
variable "workload_node_count" {
  description = "Initial node count for workload pool"
  type        = number
  default     = 2
}

variable "workload_min_node_count" {
  description = "Minimum nodes in workload pool"
  type        = number
  default     = 1
}

variable "workload_max_node_count" {
  description = "Maximum nodes in workload pool"
  type        = number
  default     = 10
}

variable "workload_machine_type" {
  description = "Machine type for workload pool"
  type        = string
  default     = "n1-standard-4"
}

variable "workload_preemptible_nodes" {
  description = "Use preemptible nodes for workload pool"
  type        = bool
  default     = false
}

variable "cluster_deletion_protection" {
  description = "Enable deletion protection on cluster"
  type        = bool
  default     = true
}

# Kubernetes Configuration
variable "kubernetes_namespace" {
  description = "Kubernetes namespace for application"
  type        = string
  default     = "default"
}

variable "kubernetes_service_account_name" {
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
  description = "Create service account key (not recommended for production)"
  type        = bool
  default     = false
  validation {
    condition     = !(var.create_service_account_key && var.environment == "prod")
    error_message = "Service account keys are blocked in production. Use Workload Identity instead."
  }
}

variable "security_pubsub_role" {
  description = "Pub/Sub IAM role granted to app workload service account"
  type        = string
  default     = "roles/pubsub.publisher"
  validation {
    condition = contains([
      "roles/pubsub.publisher",
      "roles/pubsub.subscriber",
      "roles/pubsub.viewer",
    ], var.security_pubsub_role)
    error_message = "security_pubsub_role must be one of: roles/pubsub.publisher, roles/pubsub.subscriber, roles/pubsub.viewer."
  }
  validation {
    condition = var.environment != "prod" || var.security_pubsub_role == "roles/pubsub.publisher" || (
      var.security_allow_production_iam_exceptions &&
      length(trimspace(var.security_production_iam_exception_justification)) >= 20 &&
      length(regexall("[A-Z]{2,10}-[0-9]{1,6}", var.security_production_iam_exception_justification)) > 0 &&
      length(regexall("20[0-9]{2}-[01][0-9]-[0-3][0-9]", var.security_production_iam_exception_justification)) > 0
    )
    error_message = "In production, security_pubsub_role must remain roles/pubsub.publisher unless a documented exception is enabled with a ticket and expiry date."
  }
}

variable "security_additional_permissions" {
  description = "Additional IAM project roles for app workload service account"
  type        = list(string)
  default     = []
  validation {
    condition = alltrue([
      for role in var.security_additional_permissions :
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
    error_message = "security_additional_permissions contains a deny-listed role. Remove high-privilege roles (owner/editor/IAM admin variants)."
  }
  validation {
    condition = var.environment != "prod" || length(var.security_additional_permissions) == 0 || (
      var.security_allow_production_iam_exceptions &&
      length(trimspace(var.security_production_iam_exception_justification)) >= 20 &&
      length(regexall("[A-Z]{2,10}-[0-9]{1,6}", var.security_production_iam_exception_justification)) > 0 &&
      length(regexall("20[0-9]{2}-[01][0-9]-[0-3][0-9]", var.security_production_iam_exception_justification)) > 0
    )
    error_message = "In production, additional IAM roles require security_allow_production_iam_exceptions=true and a justification with at least 20 characters including a ticket (e.g. SEC-1234) and expiry date (YYYY-MM-DD)."
  }
}

variable "security_allow_production_iam_exceptions" {
  description = "Allow additional IAM role grants in production when a documented exception is required"
  type        = bool
  default     = false
}

variable "security_production_iam_exception_justification" {
  description = "Required justification for production IAM exceptions (refer to ticket, risk acceptance, and expiry)"
  type        = string
  default     = ""
  validation {
    condition = var.environment != "prod" || !var.security_allow_production_iam_exceptions || (
      length(trimspace(var.security_production_iam_exception_justification)) >= 20 &&
      length(regexall("[A-Z]{2,10}-[0-9]{1,6}", var.security_production_iam_exception_justification)) > 0 &&
      length(regexall("20[0-9]{2}-[01][0-9]-[0-3][0-9]", var.security_production_iam_exception_justification)) > 0
    )
    error_message = "security_production_iam_exception_justification must include at least 20 characters, a ticket (e.g. SEC-1234), and an expiry date (YYYY-MM-DD) when production IAM exceptions are enabled."
  }
}

variable "security_https_egress_cidrs" {
  description = "Allowed HTTPS egress CIDRs for workload pods"
  type        = list(string)
  default = [
    "199.36.153.8/30",
    "10.0.0.0/8",
    "172.16.0.0/12",
    "192.168.0.0/16",
  ]
}

variable "security_db_egress_cidrs" {
  description = "Allowed PostgreSQL egress CIDRs for workload pods"
  type        = list(string)
  default = [
    "10.0.0.0/8",
    "172.16.0.0/12",
    "192.168.0.0/16",
  ]
}

# Database Configuration
variable "database_version" {
  description = "PostgreSQL version"
  type        = string
  default     = "POSTGRES_15"
}

variable "database_machine_tier" {
  description = "Cloud SQL machine tier"
  type        = string
  default     = "db-g1-small"
}

variable "database_availability_type" {
  description = "Database availability (REGIONAL for HA, ZONAL for single zone)"
  type        = string
  default     = "REGIONAL"
  validation {
    condition     = contains(["REGIONAL", "ZONAL"], var.database_availability_type)
    error_message = "Availability type must be 'REGIONAL' or 'ZONAL'."
  }
}

variable "database_deletion_protection" {
  description = "Enable deletion protection on database"
  type        = bool
  default     = true
}

variable "database_name" {
  description = "Database name"
  type        = string
  default     = "application"
}

variable "app_username" {
  description = "Application database user"
  type        = string
  default     = "appuser"
}

variable "create_app_user" {
  description = "Create application database user"
  type        = bool
  default     = true
}

variable "database_max_connections" {
  description = "Maximum database connections"
  type        = number
  default     = 100
}

variable "database_cpu_threshold" {
  description = "Database CPU alert threshold (%)"
  type        = number
  default     = 80
}

variable "database_storage_threshold" {
  description = "Database storage alert threshold (%)"
  type        = number
  default     = 80
}

# Storage Configuration
variable "data_retention_days" {
  description = "Data retention in days"
  type        = number
  default     = 90
}

variable "backup_retention_days" {
  description = "Backup retention in days"
  type        = number
  default     = 30
}

variable "log_retention_days" {
  description = "Log retention in days"
  type        = number
  default     = 14
}

variable "security_audit_log_retention_days" {
  description = "Retention in days for structured security audit logs"
  type        = number
  default     = 90
  validation {
    condition     = var.security_audit_log_retention_days >= 1 && var.security_audit_log_retention_days <= 3650
    error_message = "security_audit_log_retention_days must be between 1 and 3650."
  }
}

variable "backup_location" {
  description = "Backup location"
  type        = string
  default     = "us"
}

# Monitoring Configuration
variable "enable_uptime_checks" {
  description = "Enable uptime monitoring"
  type        = bool
  default     = true
}

variable "uptime_check_regions" {
  description = "Regions for uptime checks"
  type        = list(string)
  default     = ["USA", "EUROPE", "ASIA_PACIFIC"]
}

variable "app_domain" {
  description = "Application domain for health checks"
  type        = string
}

variable "secondary_app_domain" {
  description = "Secondary region application domain for health checks (optional)"
  type        = string
  default     = ""
}

variable "health_check_path" {
  description = "Health check endpoint path"
  type        = string
  default     = "/healthz"
}

variable "health_check_port" {
  description = "Health check port"
  type        = number
  default     = 8080
}

variable "pod_restart_threshold" {
  description = "Pod restart alert threshold"
  type        = number
  default     = 5
}

variable "memory_threshold" {
  description = "Memory usage alert threshold (%)"
  type        = number
  default     = 80
}

variable "cpu_threshold" {
  description = "CPU usage alert threshold (%)"
  type        = number
  default     = 80
}

variable "error_rate_threshold" {
  description = "Error rate alert threshold (%)"
  type        = number
  default     = 5
}

variable "enable_email_notifications" {
  description = "Enable email alerts"
  type        = bool
  default     = true
}

variable "alert_email" {
  description = "Email for alerts"
  type        = string
  default     = ""
}

variable "enable_slack_notifications" {
  description = "Enable Slack notifications"
  type        = bool
  default     = false
}

variable "slack_channel" {
  description = "Slack channel for alerts"
  type        = string
  default     = "#alerts"
}

variable "slack_webhook_url" {
  description = "Slack webhook URL (sensitive)"
  type        = string
  sensitive   = true
  default     = ""
  validation {
    condition = anytrue([
      var.slack_webhook_url == "",
      startswith(var.slack_webhook_url, "https://hooks.slack.com/")
    ])
    error_message = "Slack webhook URL must be valid or empty."
  }
}
