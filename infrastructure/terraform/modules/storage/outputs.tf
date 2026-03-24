output "artifacts_bucket_name" {
  description = "Artifacts bucket name"
  value       = google_storage_bucket.artifacts.name
}

output "app_data_bucket_name" {
  description = "Application data bucket name"
  value       = google_storage_bucket.app_data.name
}

output "backups_bucket_name" {
  description = "Backups bucket name"
  value       = google_storage_bucket.backups.name
}

output "logs_bucket_name" {
  description = "Logs bucket name"
  value       = google_storage_bucket.logs.name
}
