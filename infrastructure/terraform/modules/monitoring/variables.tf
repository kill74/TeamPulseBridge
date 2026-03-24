variable "workspace_name" {
  description = "Monitoring workspace name"
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

variable "region" {
  description = "GCP region"
  type        = string
}

variable "namespace" {
  description = "Kubernetes namespace"
  type        = string
  default     = "default"
}

variable "enable_uptime_checks" {
  description = "Enable uptime checks"
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

variable "log_retention_days" {
  description = "Log retention in days"
  type        = number
  default     = 30
}

variable "pod_restart_threshold" {
  description = "Pod restart alert threshold"
  type        = number
  default     = 5
}

variable "memory_threshold" {
  description = "Memory usage alert threshold percentage"
  type        = number
  default     = 80
}

variable "cpu_threshold" {
  description = "CPU usage alert threshold percentage"
  type        = number
  default     = 80
}

variable "error_rate_threshold" {
  description = "Error rate alert threshold percentage"
  type        = number
  default     = 5
}

variable "notification_channels" {
  description = "Notification channel IDs"
  type        = list(string)
  default     = []
}

variable "enable_email_notifications" {
  description = "Enable email notifications"
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
  description = "Slack channel name"
  type        = string
  default     = "#alerts"
}

variable "slack_webhook_url" {
  description = "Slack webhook URL (sensitive)"
  type        = string
  sensitive   = true
  default     = ""
}
