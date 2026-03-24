output "gke_nodes_sa_email" {
  description = "GKE nodes service account email"
  value       = google_service_account.gke_nodes.email
}

output "app_workload_sa_email" {
  description = "Application workload service account email"
  value       = google_service_account.app_workload.email
}

output "db_access_sa_email" {
  description = "Database access service account email"
  value       = google_service_account.db_access.email
}

output "pubsub_sa_email" {
  description = "Pub/Sub service account email"
  value       = google_service_account.pubsub.email
}

output "workload_identity_provider" {
  description = "Workload Identity provider"
  value       = "iam.goog/projects/${var.gcp_project}/locations/global/workloadIdentityPools/${var.gcp_project}.iam.gserviceaccount.com"
}

output "app_key" {
  description = "Application service account key (sensitive)"
  value       = try(google_service_account_key.app_key[0].private_key, null)
  sensitive   = true
}
