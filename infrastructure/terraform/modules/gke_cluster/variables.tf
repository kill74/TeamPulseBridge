variable "cluster_name" {
  description = "GKE cluster name"
  type        = string
}

variable "region" {
  description = "GCP region"
  type        = string
}

variable "environment" {
  description = "Environment name (staging, prod)"
  type        = string
}

variable "network_name" {
  description = "VPC network name"
  type        = string
}

variable "subnetwork_name" {
  description = "Subnetwork name"
  type        = string
}

variable "initial_node_count" {
  description = "Initial number of nodes in default pool"
  type        = number
  default     = 1
}

variable "min_node_count" {
  description = "Minimum nodes in default pool"
  type        = number
  default     = 1
}

variable "max_node_count" {
  description = "Maximum nodes in default pool"
  type        = number
  default     = 5
}

variable "machine_type" {
  description = "Machine type for default pool"
  type        = string
  default     = "n1-standard-2"
}

variable "disk_size_gb" {
  description = "Node disk size in GB"
  type        = number
  default     = 100
}

variable "preemptible_nodes" {
  description = "Use preemptible nodes for cost savings"
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

variable "pods_secondary_range_name" {
  description = "Secondary range name for pods"
  type        = string
  default     = "pods"
}

variable "services_secondary_range_name" {
  description = "Secondary range name for services"
  type        = string
  default     = "services"
}

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

variable "workload_disk_size_gb" {
  description = "Workload node disk size in GB"
  type        = number
  default     = 150
}

variable "workload_preemptible" {
  description = "Use preemptible nodes for workload pool"
  type        = bool
  default     = false
}

variable "network_tags" {
  description = "Network tags for nodes"
  type        = list(string)
  default     = []
}

variable "node_labels" {
  description = "Labels to apply to all nodes"
  type        = map(string)
  default     = {}
}

variable "labels" {
  description = "Labels to apply to cluster"
  type        = map(string)
  default     = {}
}

variable "deletion_protection" {
  description = "Enable deletion protection on cluster"
  type        = bool
  default     = true
}

variable "manage_kubernetes_bootstrap_resources" {
  description = "Create Kubernetes bootstrap resources (RBAC bindings) from this module"
  type        = bool
  default     = true
}

variable "generate_local_kubeconfig" {
  description = "Generate local kubeconfig helper file from cluster CA"
  type        = bool
  default     = true
}

variable "network_dependency" {
  description = "Explicit network dependency"
  type        = any
  default     = null
}
