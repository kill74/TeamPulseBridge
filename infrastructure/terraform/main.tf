provider "google" {
  project = var.gcp_project
  region  = var.region
}

locals {
  secondary_enabled     = var.enable_multi_region && var.secondary_region != ""
  secondary_region_slug = replace(var.secondary_region, "-", "")
  secondary_environment = "${var.environment}-secondary"
  secondary_app_name    = "${var.app_name}-${local.secondary_region_slug}"
  secondary_app_domain  = var.secondary_app_domain != "" ? var.secondary_app_domain : var.app_domain
}

# Kubernetes provider configuration
provider "kubernetes" {
  host                   = "https://${module.gke.cluster_endpoint}"
  cluster_ca_certificate = module.gke.cluster_ca_certificate

  exec {
    api_version = "client.authentication.k8s.io/v1beta1"
    command     = "gke-gcloud-auth-plugin"
  }
}

# Networking module
module "networking" {
  source = "./modules/networking"

  network_name      = "${var.project_name}-${var.environment}-vpc"
  region            = var.region
  environment       = var.environment
  gke_subnet_cidr   = var.gke_subnet_cidr
  pods_cidr         = var.pods_cidr
  services_cidr     = var.services_cidr
  db_subnet_cidr    = var.db_subnet_cidr
  create_db_subnet  = true
  enable_ssh_access = var.enable_ssh_access
  ssh_source_ranges = var.ssh_source_ranges

  tags = {
    environment = var.environment
    project     = var.project_name
  }
}

# GKE Cluster module
module "gke" {
  source = "./modules/gke_cluster"

  cluster_name                  = "${var.project_name}-${var.environment}"
  region                        = var.region
  environment                   = var.environment
  network_name                  = module.networking.vpc_network_name
  subnetwork_name               = module.networking.gke_subnet_name
  initial_node_count            = var.gke_initial_node_count
  min_node_count                = var.gke_min_node_count
  max_node_count                = var.gke_max_node_count
  machine_type                  = var.gke_machine_type
  disk_size_gb                  = var.gke_disk_size_gb
  preemptible_nodes             = var.gke_preemptible_nodes
  pods_secondary_range_name     = module.networking.pods_secondary_range_name
  services_secondary_range_name = module.networking.services_secondary_range_name
  cluster_min_cpu               = var.cluster_min_cpu
  cluster_max_cpu               = var.cluster_max_cpu
  cluster_min_memory            = var.cluster_min_memory
  cluster_max_memory            = var.cluster_max_memory
  workload_node_count           = var.workload_node_count
  workload_min_node_count       = var.workload_min_node_count
  workload_max_node_count       = var.workload_max_node_count
  workload_machine_type         = var.workload_machine_type
  workload_preemptible          = var.workload_preemptible_nodes
  deletion_protection           = var.cluster_deletion_protection
  network_dependency            = module.networking.vpc_network_self_link

  depends_on = [module.networking]
}

# Security module
module "security" {
  source = "./modules/security"

  cluster_name                           = module.gke.cluster_name
  app_name                               = var.app_name
  environment                            = var.environment
  gcp_project                            = var.gcp_project
  namespace                              = var.kubernetes_namespace
  ksa_name                               = var.kubernetes_service_account_name
  enable_network_policies                = var.enable_network_policies
  enable_pod_security_policy             = var.enable_pod_security_policy
  create_service_account_key             = var.create_service_account_key
  pubsub_role                            = var.security_pubsub_role
  permissions                            = var.security_additional_permissions
  allow_production_iam_exceptions        = var.security_allow_production_iam_exceptions
  production_iam_exception_justification = var.security_production_iam_exception_justification
  https_egress_cidrs                     = var.security_https_egress_cidrs
  db_egress_cidrs                        = var.security_db_egress_cidrs

  depends_on = [module.gke]
}

# Storage module
module "storage" {
  source = "./modules/storage"

  gcp_project               = var.gcp_project
  region                    = var.region
  environment               = var.environment
  app_service_account_email = module.security.app_workload_sa_email
  data_retention_days       = var.data_retention_days
  backup_retention_days     = var.backup_retention_days
  log_retention_days        = var.log_retention_days

  labels = {
    environment = var.environment
    project     = var.project_name
  }
}

# Database module
module "database" {
  source = "./modules/database"

  instance_name         = "${var.project_name}-${var.environment}-db"
  region                = var.region
  environment           = var.environment
  database_version      = var.database_version
  machine_tier          = var.database_machine_tier
  availability_type     = var.database_availability_type
  deletion_protection   = var.database_deletion_protection
  backup_location       = var.backup_location
  database_name         = var.database_name
  app_username          = var.app_username
  create_app_user       = var.create_app_user
  iam_service_account   = module.security.app_workload_sa_email
  network_id            = module.networking.vpc_network_id
  max_connections       = var.database_max_connections
  gcp_project           = var.gcp_project
  cpu_threshold         = var.database_cpu_threshold
  storage_threshold     = var.database_storage_threshold
  notification_channels = module.monitoring.notification_channel_ids

  depends_on = [
    module.networking,
    module.security
  ]
}

# Monitoring module
module "monitoring" {
  source = "./modules/monitoring"

  workspace_name             = "${var.project_name}-${var.environment}-monitoring"
  app_name                   = var.app_name
  environment                = var.environment
  gcp_project                = var.gcp_project
  region                     = var.region
  namespace                  = var.kubernetes_namespace
  enable_uptime_checks       = var.enable_uptime_checks
  uptime_check_regions       = var.uptime_check_regions
  app_domain                 = var.app_domain
  health_check_path          = var.health_check_path
  health_check_port          = var.health_check_port
  log_retention_days         = var.log_retention_days
  pod_restart_threshold      = var.pod_restart_threshold
  memory_threshold           = var.memory_threshold
  cpu_threshold              = var.cpu_threshold
  error_rate_threshold       = var.error_rate_threshold
  enable_email_notifications = var.enable_email_notifications
  alert_email                = var.alert_email
  enable_slack_notifications = var.enable_slack_notifications
  slack_channel              = var.slack_channel
  slack_webhook_url          = var.slack_webhook_url

  depends_on = [module.gke]
}

# Secondary region networking
module "networking_secondary" {
  count  = local.secondary_enabled ? 1 : 0
  source = "./modules/networking"

  network_name      = "${var.project_name}-${var.environment}-${local.secondary_region_slug}-vpc"
  region            = var.secondary_region
  environment       = local.secondary_environment
  gke_subnet_cidr   = var.secondary_gke_subnet_cidr
  pods_cidr         = var.secondary_pods_cidr
  services_cidr     = var.secondary_services_cidr
  db_subnet_cidr    = var.secondary_db_subnet_cidr
  create_db_subnet  = true
  enable_ssh_access = var.enable_ssh_access
  ssh_source_ranges = var.ssh_source_ranges

  tags = {
    environment = local.secondary_environment
    project     = var.project_name
    region      = var.secondary_region
  }
}

# Secondary region GKE cluster
module "gke_secondary" {
  count  = local.secondary_enabled ? 1 : 0
  source = "./modules/gke_cluster"

  cluster_name                          = "${var.project_name}-${var.environment}-${local.secondary_region_slug}"
  region                                = var.secondary_region
  environment                           = local.secondary_environment
  network_name                          = module.networking_secondary[0].vpc_network_name
  subnetwork_name                       = module.networking_secondary[0].gke_subnet_name
  initial_node_count                    = var.gke_initial_node_count
  min_node_count                        = var.gke_min_node_count
  max_node_count                        = var.gke_max_node_count
  machine_type                          = var.gke_machine_type
  disk_size_gb                          = var.gke_disk_size_gb
  preemptible_nodes                     = var.gke_preemptible_nodes
  pods_secondary_range_name             = module.networking_secondary[0].pods_secondary_range_name
  services_secondary_range_name         = module.networking_secondary[0].services_secondary_range_name
  cluster_min_cpu                       = var.cluster_min_cpu
  cluster_max_cpu                       = var.cluster_max_cpu
  cluster_min_memory                    = var.cluster_min_memory
  cluster_max_memory                    = var.cluster_max_memory
  workload_node_count                   = var.workload_node_count
  workload_min_node_count               = var.workload_min_node_count
  workload_max_node_count               = var.workload_max_node_count
  workload_machine_type                 = var.workload_machine_type
  workload_preemptible                  = var.workload_preemptible_nodes
  manage_kubernetes_bootstrap_resources = false
  generate_local_kubeconfig             = false
  deletion_protection                   = var.cluster_deletion_protection
  network_dependency                    = module.networking_secondary[0].vpc_network_self_link

  depends_on = [module.networking_secondary]
}

# Secondary region security
module "security_secondary" {
  count  = local.secondary_enabled ? 1 : 0
  source = "./modules/security"

  cluster_name                           = module.gke_secondary[0].cluster_name
  app_name                               = local.secondary_app_name
  environment                            = local.secondary_environment
  gcp_project                            = var.gcp_project
  namespace                              = var.kubernetes_namespace
  ksa_name                               = var.kubernetes_service_account_name
  enable_network_policies                = var.enable_network_policies
  enable_pod_security_policy             = var.enable_pod_security_policy
  manage_kubernetes_security_resources   = false
  create_service_account_key             = var.create_service_account_key
  pubsub_role                            = var.security_pubsub_role
  permissions                            = var.security_additional_permissions
  allow_production_iam_exceptions        = var.security_allow_production_iam_exceptions
  production_iam_exception_justification = var.security_production_iam_exception_justification
  https_egress_cidrs                     = var.security_https_egress_cidrs
  db_egress_cidrs                        = var.security_db_egress_cidrs

  depends_on = [module.gke_secondary]
}

# Secondary region storage
module "storage_secondary" {
  count  = local.secondary_enabled ? 1 : 0
  source = "./modules/storage"

  gcp_project               = var.gcp_project
  region                    = var.secondary_region
  environment               = local.secondary_environment
  app_service_account_email = module.security_secondary[0].app_workload_sa_email
  data_retention_days       = var.data_retention_days
  backup_retention_days     = var.backup_retention_days
  log_retention_days        = var.log_retention_days

  labels = {
    environment = local.secondary_environment
    project     = var.project_name
    region      = var.secondary_region
  }
}

# Secondary region database
module "database_secondary" {
  count  = local.secondary_enabled ? 1 : 0
  source = "./modules/database"

  instance_name         = "${var.project_name}-${var.environment}-${local.secondary_region_slug}-db"
  region                = var.secondary_region
  environment           = local.secondary_environment
  database_version      = var.database_version
  machine_tier          = var.database_machine_tier
  availability_type     = var.database_availability_type
  deletion_protection   = var.database_deletion_protection
  backup_location       = var.backup_location
  database_name         = var.database_name
  app_username          = var.app_username
  create_app_user       = var.create_app_user
  iam_service_account   = module.security_secondary[0].app_workload_sa_email
  network_id            = module.networking_secondary[0].vpc_network_id
  max_connections       = var.database_max_connections
  gcp_project           = var.gcp_project
  cpu_threshold         = var.database_cpu_threshold
  storage_threshold     = var.database_storage_threshold
  notification_channels = module.monitoring_secondary[0].notification_channel_ids

  depends_on = [
    module.networking_secondary,
    module.security_secondary,
    module.monitoring_secondary,
  ]
}

# Secondary region monitoring
module "monitoring_secondary" {
  count  = local.secondary_enabled ? 1 : 0
  source = "./modules/monitoring"

  workspace_name             = "${var.project_name}-${var.environment}-${local.secondary_region_slug}-monitoring"
  app_name                   = local.secondary_app_name
  environment                = local.secondary_environment
  gcp_project                = var.gcp_project
  region                     = var.secondary_region
  namespace                  = var.kubernetes_namespace
  enable_uptime_checks       = var.enable_uptime_checks
  uptime_check_regions       = var.uptime_check_regions
  app_domain                 = local.secondary_app_domain
  health_check_path          = var.health_check_path
  health_check_port          = var.health_check_port
  log_retention_days         = var.log_retention_days
  pod_restart_threshold      = var.pod_restart_threshold
  memory_threshold           = var.memory_threshold
  cpu_threshold              = var.cpu_threshold
  error_rate_threshold       = var.error_rate_threshold
  enable_email_notifications = var.enable_email_notifications
  alert_email                = var.alert_email
  enable_slack_notifications = var.enable_slack_notifications
  slack_channel              = var.slack_channel
  slack_webhook_url          = var.slack_webhook_url

  depends_on = [module.gke_secondary]
}
