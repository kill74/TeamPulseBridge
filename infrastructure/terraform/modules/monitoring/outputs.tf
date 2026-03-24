output "notification_channel_ids" {
  description = "Notification channel IDs"
  value = concat(
    try([google_monitoring_notification_channel.email[0].id], []),
    try([google_monitoring_notification_channel.slack[0].id], [])
  )
}

output "dashboard_id" {
  description = "Monitoring dashboard ID"
  value       = google_monitoring_dashboard.cluster.id
}

output "uptime_check_id" {
  description = "Uptime check ID"
  value       = try(google_monitoring_uptime_check_config.app_health[0].id, null)
}
