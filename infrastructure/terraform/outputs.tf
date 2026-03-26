# Cluster Outputs
output "cluster_name" {
  description = "GKE cluster name"
  value       = module.gke.cluster_name
}

output "cluster_location" {
  description = "GKE cluster location"
  value       = module.gke.cluster_location
}

output "cluster_endpoint" {
  description = "GKE cluster endpoint"
  value       = module.gke.cluster_endpoint
  sensitive   = true
}

output "kubernetes_cluster_host" {
  description = "Kubernetes cluster host for provider configuration"
  value       = module.gke.kubernetes_cluster_host
  sensitive   = true
}

# Network Outputs
output "vpc_network_name" {
  description = "VPC network name"
  value       = module.networking.vpc_network_name
}

output "gke_subnet_name" {
  description = "GKE subnet name"
  value       = module.networking.gke_subnet_name
}

# Database Outputs
output "database_connection_name" {
  description = "Cloud SQL connection name"
  value       = module.database.instance_connection_name
}

output "database_private_ip" {
  description = "Cloud SQL private IP"
  value       = module.database.instance_private_ip
}

output "database_name" {
  description = "Database name"
  value       = module.database.database_name
}

output "app_database_user" {
  description = "Application database user"
  value       = module.database.app_username
}

# Security Outputs
output "app_service_account_email" {
  description = "Application service account email"
  value       = module.security.app_workload_sa_email
}

output "database_service_account_email" {
  description = "Database service account email"
  value       = module.security.db_access_sa_email
}

# Storage Outputs
output "artifacts_bucket" {
  description = "Artifacts bucket name"
  value       = module.storage.artifacts_bucket_name
}

output "app_data_bucket" {
  description = "Application data bucket name"
  value       = module.storage.app_data_bucket_name
}

output "backups_bucket" {
  description = "Backups bucket name"
  value       = module.storage.backups_bucket_name
}

output "logs_bucket" {
  description = "Logs bucket name"
  value       = module.storage.logs_bucket_name
}

# Monitoring Outputs
output "dashboard_url" {
  description = "Monitoring dashboard URL"
  value       = "https://console.cloud.google.com/monitoring/dashboards/custom/${module.monitoring.dashboard_id}?project=${var.gcp_project}"
}

# Summary
output "deployment_summary" {
  description = "Deployment summary"
  value = {
    environment              = var.environment
    region                   = var.region
    cluster_name             = module.gke.cluster_name
    database_instance        = module.database.instance_name
    network_vpc              = module.networking.vpc_network_name
    app_service_account      = module.security.app_workload_sa_email
    artifacts_bucket         = module.storage.artifacts_bucket_name
  }
}

output "multi_region_enabled" {
  description = "Whether active-active multi-region topology is enabled"
  value       = var.enable_multi_region && var.secondary_region != ""
}

output "secondary_cluster_name" {
  description = "Secondary GKE cluster name when multi-region is enabled"
  value       = var.enable_multi_region && var.secondary_region != "" ? module.gke_secondary[0].cluster_name : null
}

output "secondary_cluster_location" {
  description = "Secondary GKE cluster location when multi-region is enabled"
  value       = var.enable_multi_region && var.secondary_region != "" ? module.gke_secondary[0].cluster_location : null
}

output "secondary_database_connection_name" {
  description = "Secondary Cloud SQL connection name when multi-region is enabled"
  value       = var.enable_multi_region && var.secondary_region != "" ? module.database_secondary[0].instance_connection_name : null
}

output "active_active_topology" {
  description = "Primary and secondary region topology summary"
  value = {
    primary = {
      region       = var.region
      cluster_name = module.gke.cluster_name
      app_domain   = var.app_domain
    }
    secondary = var.enable_multi_region && var.secondary_region != "" ? {
      region       = var.secondary_region
      cluster_name = module.gke_secondary[0].cluster_name
      app_domain   = var.secondary_app_domain != "" ? var.secondary_app_domain : var.app_domain
    } : null
  }
}
