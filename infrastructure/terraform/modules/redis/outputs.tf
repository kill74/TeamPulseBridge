output "host" {
  description = "The IP address of the instance."
  value       = google_redis_instance.cache.host
}

output "port" {
  description = "The port number of the instance."
  value       = google_redis_instance.cache.port
}

output "auth_string" {
  description = "AUTH String set on the instance. This field will only be populated if auth_enabled is true."
  value       = google_redis_instance.cache.auth_string
  sensitive   = true
}
