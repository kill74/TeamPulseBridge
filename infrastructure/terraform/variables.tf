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

# Terraform Cloud
variable "terraform_cloud_org" {
  description = "Terraform Cloud organization"
  type        = string
  default     = ""
}

# Network Configuration
variable "gke_subnet_cidr" {
  description = "CIDR for GKE subnet"
  type        = string
  default     = "10.0.0.0/24"
}

variable "pods_cidr" {
  description = "CIDR for pods secondary range"
  type        = string
  default     = "10.1.0.0/16"
}

variable "services_cidr" {
  description = "CIDR for services secondary range"
  type        = string
  default     = "10.2.0.0/16"
}

variable "db_subnet_cidr" {
  description = "CIDR for database subnet"
  type        = string
  default     = "10.3.0.0/24"
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
