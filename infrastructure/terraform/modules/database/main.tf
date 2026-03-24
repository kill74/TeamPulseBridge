terraform {
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 5.0"
    }
    random = {
      source  = "hashicorp/random"
      version = "~> 3.0"
    }
  }
}

# Random suffix for database instance name
resource "random_string" "db_suffix" {
  length  = 4
  special = false
  upper   = false
}

# Cloud SQL instance
resource "google_sql_database_instance" "instance" {
  name             = "${var.instance_name}-${random_string.db_suffix.result}"
  database_version = var.database_version
  region           = var.region
  deletion_protection = var.deletion_protection

  settings {
    tier                        = var.machine_tier
    availability_type           = var.availability_type
    backup_configuration {
      enabled                        = true
      point_in_time_recovery_enabled = true
      transaction_log_retention_days = 7
      backup_kind                    = "AUTOMATED"
      start_time                     = var.backup_start_time
      location                       = var.backup_location
    }

    ip_configuration {
      ipv4_enabled                                  = false
      private_network                               = var.network_id
      enable_private_path_for_cloudsql_cloud_sql    = true
      require_ssl                                   = true

      database_flags {
        name  = "cloudsql_iam_authentication"
        value = "on"
      }
    }

    database_flags {
      name  = "max_connections"
      value = var.max_connections
    }

    database_flags {
      name  = "log_checkpoints"
      value = "on"
    }

    database_flags {
      name  = "log_connections"
      value = "on"
    }

    maintenance_window {
      day          = 7  # Sunday
      hour         = 3
      update_track = "stable"
    }

    insights_config {
      query_insights_enabled  = true
      query_string_length     = 1024
      record_application_tags = true
    }

    backup_configuration {
      location = var.backup_location
    }

    user_labels = merge(
      var.labels,
      {
        "environment" = var.environment
        "managed-by"  = "terraform"
      }
    )
  }

  timeouts {
    create = "15m"
    update = "15m"
    delete = "15m"
  }
}

# Database user with IAM authentication
resource "google_sql_user" "iam_user" {
  name     = var.iam_service_account
  instance = google_sql_database_instance.instance.name
  type     = "CLOUD_IAM_SERVICE_ACCOUNT"
}

# Application database
resource "google_sql_database" "database" {
  name     = var.database_name
  instance = google_sql_database_instance.instance.name
}

# Application user with password authentication (for local development)
resource "random_password" "db_password" {
  length  = 32
  special = true
}

resource "google_sql_user" "app_user" {
  count    = var.create_app_user ? 1 : 0
  name     = var.app_username
  instance = google_sql_database_instance.instance.name
  password = random_password.db_password.result
  type     = "BUILT_IN"
}

# Database backup schedule
resource "google_sql_backup_run" "backup" {
  instance = google_sql_database_instance.instance.name

  depends_on = [google_sql_database_instance.instance]
}

# Monitoring alert for high CPU
resource "google_monitoring_alert_policy" "high_cpu" {
  display_name = "${var.instance_name}-high-cpu"
  combiner     = "OR"

  conditions {
    display_name = "SQL CPU usage > ${var.cpu_threshold}%"

    condition_threshold {
      filter          = "resource.type = \"cloudsql_database\" AND resource.label.database_id = \"${var.gcp_project}:${google_sql_database_instance.instance.name}\" AND metric.type = \"cloudsql.googleapis.com/database/cpu/utilization\""
      duration        = "300s"
      comparison      = "COMPARISON_GT"
      threshold_value = var.cpu_threshold / 100

      aggregations {
        alignment_period  = "60s"
        per_series_aligner = "ALIGN_MEAN"
      }
    }
  }

  notification_channels = var.notification_channels

  alert_strategy {
    auto_close = "1800s"
  }
}

# Monitoring alert for low storage
resource "google_monitoring_alert_policy" "low_storage" {
  display_name = "${var.instance_name}-low-storage"
  combiner     = "OR"

  conditions {
    display_name = "SQL storage < ${var.storage_threshold}%"

    condition_threshold {
      filter          = "resource.type = \"cloudsql_database\" AND resource.label.database_id = \"${var.gcp_project}:${google_sql_database_instance.instance.name}\" AND metric.type = \"cloudsql.googleapis.com/database/disk/utilization\""
      duration        = "300s"
      comparison      = "COMPARISON_GT"
      threshold_value = var.storage_threshold / 100

      aggregations {
        alignment_period  = "60s"
        per_series_aligner = "ALIGN_MEAN"
      }
    }
  }

  notification_channels = var.notification_channels

  alert_strategy {
    auto_close = "1800s"
  }
}
