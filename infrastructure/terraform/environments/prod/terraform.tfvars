# Production Environment Configuration
gcp_project         = "your-gcp-project-id"
region              = "us-central1"
environment         = "prod"
project_name        = "teampulse"
app_name            = "ingestion-gateway"
enable_multi_region = true
secondary_region    = "us-east1"

# Network
gke_subnet_cidr           = "10.0.0.0/24"
pods_cidr                 = "10.1.0.0/16"
services_cidr             = "10.2.0.0/16"
secondary_gke_subnet_cidr = "10.10.0.0/24"
secondary_pods_cidr       = "10.11.0.0/16"
secondary_services_cidr   = "10.12.0.0/16"
secondary_db_subnet_cidr  = "10.13.0.0/24"

# GKE Configuration (HA-enabled for production)
gke_initial_node_count = 3
gke_min_node_count     = 3
gke_max_node_count     = 20
gke_machine_type       = "n1-standard-4"
gke_preemptible_nodes  = false

workload_node_count        = 3
workload_min_node_count    = 2
workload_max_node_count    = 20
workload_machine_type      = "n1-standard-4"
workload_preemptible_nodes = false

cluster_deletion_protection = true

# Database (HA enabled for production)
database_version             = "POSTGRES_15"
database_machine_tier        = "db-custom-4-16384" # 4 CPU, 16GB RAM
database_availability_type   = "REGIONAL"
database_deletion_protection = true

# Storage
data_retention_days   = 90
backup_retention_days = 30
log_retention_days    = 30

# Application domain
app_domain           = "api.example.com"
secondary_app_domain = "api-us-east1.example.com"

# Monitoring (all enabled for production)
enable_uptime_checks       = true
uptime_check_regions       = ["USA", "EUROPE", "ASIA_PACIFIC"]
enable_email_notifications = true
alert_email                = "devops-alerts@example.com"
enable_slack_notifications = true
slack_channel              = "#prod-alerts"

# Security (strict in production)
enable_ssh_access               = false
enable_network_policies         = true
enable_pod_security_policy      = true
create_service_account_key      = false
security_pubsub_role            = "roles/pubsub.publisher"
security_additional_permissions = []
# Production IAM exceptions are blocked by default.
# Enable only with approved risk acceptance and bounded expiry.
# security_allow_production_iam_exceptions = true
# security_additional_permissions = ["roles/pubsub.subscriber"]
# security_production_iam_exception_justification = "SEC-1234 temporary subscriber access for incident replay until 2026-12-31"
security_https_egress_cidrs = [
  "199.36.153.8/30",
  "10.0.0.0/8",
  "172.16.0.0/12",
  "192.168.0.0/16",
]
security_db_egress_cidrs = [
  "10.0.0.0/8",
  "172.16.0.0/12",
  "192.168.0.0/16",
]
