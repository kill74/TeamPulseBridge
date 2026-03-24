variable "gcp_project" {
  description = "GCP project ID"
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

variable "app_service_account_email" {
  description = "Application service account email"
  type        = string
}

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

variable "labels" {
  description = "Labels to apply to resources"
  type        = map(string)
  default     = {}
}
