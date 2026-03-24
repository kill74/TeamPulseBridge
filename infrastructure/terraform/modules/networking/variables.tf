variable "network_name" {
  description = "VPC network name"
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

variable "gke_subnet_cidr" {
  description = "CIDR range for GKE subnet"
  type        = string
  default     = "10.0.0.0/24"
}

variable "pods_cidr" {
  description = "CIDR range for pod secondary IP range"
  type        = string
  default     = "10.1.0.0/16"
}

variable "services_cidr" {
  description = "CIDR range for services secondary IP range"
  type        = string
  default     = "10.2.0.0/16"
}

variable "db_subnet_cidr" {
  description = "CIDR range for database subnet"
  type        = string
  default     = "10.3.0.0/24"
}

variable "create_db_subnet" {
  description = "Whether to create database subnet"
  type        = bool
  default     = true
}

variable "enable_ssh_access" {
  description = "Enable SSH access to nodes"
  type        = bool
  default     = false
}

variable "ssh_source_ranges" {
  description = "CIDR ranges allowed for SSH access"
  type        = list(string)
  default     = []
}

variable "enable_cloud_armor" {
  description = "Enable Cloud Armor protection"
  type        = bool
  default     = true
}

variable "tags" {
  description = "Tags to apply to resources"
  type        = map(string)
  default     = {}
}
