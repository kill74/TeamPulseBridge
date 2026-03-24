variable "instance_name" {
  description = "Cloud SQL instance name"
  type        = string
}

variable "region" {
  description = "GCP region"
  type        = string
}

variable "environment" {
  description = "Environment name"
  type        = string
}

variable "database_version" {
  description = "PostgreSQL version"
  type        = string
  default     = "POSTGRES_15"
}

variable "machine_tier" {
  description = "Cloud SQL machine tier"
  type        = string
  default     = "db-g1-small"
}

variable "availability_type" {
  description = "Availability type (REGIONAL for HA, ZONAL for single zone)"
  type        = string
  default     = "REGIONAL"
}

variable "deletion_protection" {
  description = "Enable deletion protection"
  type        = bool
  default     = true
}

variable "backup_start_time" {
  description = "Backup start time (HH:MM format, UTC)"
  type        = string
  default     = "02:00"
}

variable "backup_location" {
  description = "Location for backups"
  type        = string
  default     = "us"
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
  description = "Create application user"
  type        = bool
  default     = true
}

variable "iam_service_account" {
  description = "IAM service account email for SQL auth"
  type        = string
}

variable "network_id" {
  description = "VPC network ID for private IP"
  type        = string
}

variable "max_connections" {
  description = "Maximum database connections"
  type        = number
  default     = 100
}

variable "gcp_project" {
  description = "GCP project ID"
  type        = string
}

variable "cpu_threshold" {
  description = "CPU alert threshold percentage"
  type        = number
  default     = 80
}

variable "storage_threshold" {
  description = "Storage alert threshold percentage"
  type        = number
  default     = 80
}

variable "notification_channels" {
  description = "Notification channels for alerts"
  type        = list(string)
  default     = []
}

variable "labels" {
  description = "Labels to apply to resources"
  type        = map(string)
  default     = {}
}
