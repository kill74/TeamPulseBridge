terraform {
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 5.0"
    }
  }
}

# Artifact bucket for container images
resource "google_storage_bucket" "artifacts" {
  name          = "${var.gcp_project}-artifacts-${var.environment}"
  location      = var.region
  force_destroy = false

  uniform_bucket_level_access = true

  versioning {
    enabled = true
  }

  lifecycle_rule {
    condition {
      num_newer_versions = 3
    }

    action {
      type = "Delete"
    }
  }

  lifecycle_rule {
    condition {
      age = 30
    }

    action {
      type = "Delete"
    }
  }

  labels = merge(
    var.labels,
    {
      "environment" = var.environment
      "purpose"     = "artifacts"
    }
  )
}

# Application data bucket
resource "google_storage_bucket" "app_data" {
  name          = "${var.gcp_project}-app-data-${var.environment}"
  location      = var.region
  force_destroy = false

  uniform_bucket_level_access = true

  versioning {
    enabled = true
  }

  lifecycle_rule {
    condition {
      age = var.data_retention_days
    }

    action {
      type          = "SetStorageClass"
      storage_class = "COLDLINE"
    }
  }

  labels = merge(
    var.labels,
    {
      "environment" = var.environment
      "purpose"     = "application-data"
    }
  )
}

# Backup bucket
resource "google_storage_bucket" "backups" {
  name          = "${var.gcp_project}-backups-${var.environment}"
  location      = var.region
  force_destroy = false

  uniform_bucket_level_access = true

  versioning {
    enabled = true
  }

  lifecycle_rule {
    condition {
      age = var.backup_retention_days
    }

    action {
      type = "Delete"
    }
  }

  labels = merge(
    var.labels,
    {
      "environment" = var.environment
      "purpose"     = "backups"
    }
  )
}

# Logs bucket
resource "google_storage_bucket" "logs" {
  name          = "${var.gcp_project}-logs-${var.environment}"
  location      = var.region
  force_destroy = false

  uniform_bucket_level_access = true

  versioning {
    enabled = false
  }

  lifecycle_rule {
    condition {
      age = var.log_retention_days
    }

    action {
      type = "Delete"
    }
  }

  labels = merge(
    var.labels,
    {
      "environment" = var.environment
      "purpose"     = "logs"
    }
  )
}

# Bucket IAM binding for artifacts
resource "google_storage_bucket_iam_member" "artifacts_viewer" {
  bucket = google_storage_bucket.artifacts.name
  role   = "roles/storage.objectViewer"
  member = "serviceAccount:${var.app_service_account_email}"
}

# Bucket IAM binding for app data
resource "google_storage_bucket_iam_member" "app_data_admin" {
  bucket = google_storage_bucket.app_data.name
  role   = "roles/storage.objectAdmin"
  member = "serviceAccount:${var.app_service_account_email}"
}

# Bucket IAM binding for backups
resource "google_storage_bucket_iam_member" "backups_admin" {
  bucket = google_storage_bucket.backups.name
  role   = "roles/storage.objectAdmin"
  member = "serviceAccount:${var.app_service_account_email}"
}

# Bucket IAM binding for logs
resource "google_storage_bucket_iam_member" "logs_writer" {
  bucket = google_storage_bucket.logs.name
  role   = "roles/storage.objectCreator"
  member = "serviceAccount:${var.app_service_account_email}"
}

# Enable public access prevention on buckets
resource "google_storage_bucket_public_access_prevention" "artifacts_prevention" {
  bucket                      = google_storage_bucket.artifacts.name
  public_access_prevention    = "enforced"
}

resource "google_storage_bucket_public_access_prevention" "app_data_prevention" {
  bucket                      = google_storage_bucket.app_data.name
  public_access_prevention    = "enforced"
}

resource "google_storage_bucket_public_access_prevention" "backups_prevention" {
  bucket                      = google_storage_bucket.backups.name
  public_access_prevention    = "enforced"
}

resource "google_storage_bucket_public_access_prevention" "logs_prevention" {
  bucket                      = google_storage_bucket.logs.name
  public_access_prevention    = "enforced"
}
