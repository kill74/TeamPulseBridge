# Staging Environment Configuration
gcp_project = "your-gcp-project-id"
region      = "us-central1"
environment = "staging"
project_name = "teampulse"
app_name    = "ingestion-gateway"

# Network
gke_subnet_cidr = "10.0.0.0/24"
pods_cidr       = "10.1.0.0/16"
services_cidr   = "10.2.0.0/16"

# GKE Configuration (cost-optimized for staging)
gke_initial_node_count = 1
gke_min_node_count     = 1
gke_max_node_count     = 3
gke_machine_type       = "n1-standard-2"
gke_preemptible_nodes  = true

workload_node_count        = 1
workload_min_node_count    = 1
workload_max_node_count    = 5
workload_machine_type      = "n1-standard-2"
workload_preemptible_nodes = true

cluster_deletion_protection = false

# Database (single zone for staging)
database_version            = "POSTGRES_15"
database_machine_tier       = "db-g1-small"
database_availability_type  = "ZONAL"
database_deletion_protection = false

# Storage
data_retention_days    = 30
backup_retention_days  = 7
log_retention_days     = 7

# Application domain
app_domain = "staging.api.example.com"

# Monitoring
enable_uptime_checks = false
enable_email_notifications = true
alert_email = "devops-staging@example.com"
enable_slack_notifications = false

# Security
enable_ssh_access = false
enable_network_policies = true
enable_pod_security_policy = false  # Less strict in staging
create_service_account_key = false
