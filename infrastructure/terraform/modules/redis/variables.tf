variable "instance_name" {
  type        = string
  description = "The ID of the instance or a fully qualified identifier for the instance."
}

variable "tier" {
  type        = string
  description = "The service tier of the instance. Must be one of these values: BASIC, STANDARD_HA"
  default     = "STANDARD_HA"
}

variable "memory_size_gb" {
  type        = number
  description = "Redis memory size in GiB."
  default     = 1
}

variable "region" {
  type        = string
  description = "The name of the Redis region of the instance."
}

variable "environment" {
  type        = string
  description = "Environment name (e.g., prod, staging)"
}

variable "network_id" {
  type        = string
  description = "The full name of the Google Compute Engine network to which the instance is connected."
}
