output "cluster_name" {
  description = "GKE cluster name"
  value       = google_container_cluster.primary.name
}

output "cluster_location" {
  description = "GKE cluster location"
  value       = google_container_cluster.primary.location
}

output "cluster_endpoint" {
  description = "GKE cluster endpoint"
  value       = google_container_cluster.primary.endpoint
  sensitive   = true
}

output "cluster_ca_certificate" {
  description = "GKE cluster CA certificate"
  value       = base64decode(google_container_cluster.primary.master_auth[0].cluster_ca_certificate)
  sensitive   = true
}

output "kubernetes_cluster_name" {
  description = "Kubernetes cluster name for kubeconfig"
  value       = google_container_cluster.primary.name
}

output "region" {
  description = "GCP region"
  value       = var.region
}

output "project_id" {
  description = "GCP project ID"
  value       = google_container_cluster.primary.project
}

output "workload_pool_name" {
  description = "Workload node pool name"
  value       = google_container_node_pool.workload_pool.name
}

output "kubernetes_cluster_host" {
  description = "Kubernetes cluster host for provider config"
  value       = "https://${google_container_cluster.primary.endpoint}"
  sensitive   = true
}
