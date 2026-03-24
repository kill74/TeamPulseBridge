output "instance_name" {
  description = "Cloud SQL instance name"
  value       = google_sql_database_instance.instance.name
}

output "instance_connection_name" {
  description = "Cloud SQL instance connection name"
  value       = google_sql_database_instance.instance.connection_name
}

output "instance_private_ip" {
  description = "Cloud SQL private IP address"
  value       = try(google_sql_database_instance.instance.private_ip_address, null)
}

output "database_name" {
  description = "Database name"
  value       = google_sql_database.database.name
}

output "app_username" {
  description = "Application user name"
  value       = try(google_sql_user.app_user[0].name, null)
}

output "app_password" {
  description = "Application user password (initial)"
  value       = try(google_sql_user.app_user[0].password, null)
  sensitive   = true
}

output "iam_user" {
  description = "IAM user email"
  value       = google_sql_user.iam_user.name
}

output "database_version" {
  description = "PostgreSQL version"
  value       = google_sql_database_instance.instance.database_version
}

output "self_link" {
  description = "Cloud SQL instance self link"
  value       = google_sql_database_instance.instance.self_link
}
