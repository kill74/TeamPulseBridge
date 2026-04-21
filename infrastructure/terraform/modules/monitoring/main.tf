terraform {
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 5.0"
    }
  }
}

# Uptime check for application health
resource "google_monitoring_uptime_check_config" "app_health" {
  count            = var.enable_uptime_checks ? 1 : 0
  display_name     = "${var.app_name}-health-check"
  timeout          = "10s"
  period           = "60s"
  selected_regions = var.uptime_check_regions

  http_check {
    path           = var.health_check_path
    port           = var.health_check_port
    use_ssl        = true
    request_method = "GET"
  }

  monitored_resource {
    type = "uptime_url"
    labels = {
      host = var.app_domain
    }
  }
}

# Log sink for error logs
resource "google_logging_project_sink" "error_logs" {
  name        = "${var.app_name}-error-logs"
  destination = "logging.googleapis.com/projects/${var.gcp_project}/locations/${var.region}/buckets/error-logs"

  filter = "severity >= ERROR"

  unique_writer_identity = true
}

# Log sink for application logs
resource "google_logging_project_sink" "app_logs" {
  name        = "${var.app_name}-app-logs"
  destination = "logging.googleapis.com/projects/${var.gcp_project}/locations/${var.region}/buckets/app-logs"

  filter = "resource.type = \"k8s_container\" AND resource.labels.namespace_name = \"${var.namespace}\""

  unique_writer_identity = true
}

# Log sink for structured security audit events
resource "google_logging_project_sink" "security_audit_logs" {
  name        = "${var.app_name}-security-audit-logs"
  destination = "logging.googleapis.com/projects/${var.gcp_project}/locations/${var.region}/buckets/security-audit-logs"

  filter = "resource.type = \"k8s_container\" AND resource.labels.namespace_name = \"${var.namespace}\" AND jsonPayload.audit_stream = \"security\""

  unique_writer_identity = true
}

# Log bucket for storing logs
resource "google_logging_project_bucket_config" "error_bucket" {
  project        = var.gcp_project
  location       = var.region
  bucket_id      = "error-logs"
  retention_days = var.log_retention_days
}

resource "google_logging_project_bucket_config" "app_bucket" {
  project        = var.gcp_project
  location       = var.region
  bucket_id      = "app-logs"
  retention_days = var.log_retention_days
}

resource "google_logging_project_bucket_config" "security_audit_bucket" {
  project        = var.gcp_project
  location       = var.region
  bucket_id      = "security-audit-logs"
  retention_days = var.security_audit_log_retention_days
}

# Alert policy for pod restarts
resource "google_monitoring_alert_policy" "pod_restarts" {
  display_name = "${var.app_name}-high-pod-restarts"
  combiner     = "OR"

  conditions {
    display_name = "Pod restart rate > ${var.pod_restart_threshold} per minute"

    condition_threshold {
      filter          = "resource.type = \"k8s_pod\" AND metric.type = \"kubernetes.io/pod/restart_count\""
      duration        = "300s"
      comparison      = "COMPARISON_GT"
      threshold_value = var.pod_restart_threshold

      aggregations {
        alignment_period   = "60s"
        per_series_aligner = "ALIGN_RATE"
      }
    }
  }

  notification_channels = var.notification_channels
  alert_strategy {
    auto_close = "1800s"
  }
}

# Alert policy for node conditions
resource "google_monitoring_alert_policy" "node_condition" {
  display_name = "${var.app_name}-node-not-ready"
  combiner     = "OR"

  conditions {
    display_name = "Node condition: NotReady"

    condition_threshold {
      filter          = "resource.type = \"k8s_node\" AND metric.type = \"kubernetes.io/node/condition\" AND resource.label.condition_type = \"Ready\""
      duration        = "300s"
      comparison      = "COMPARISON_LT"
      threshold_value = 1

      aggregations {
        alignment_period   = "60s"
        per_series_aligner = "ALIGN_FRACTION_TRUE"
      }
    }
  }

  notification_channels = var.notification_channels
  alert_strategy {
    auto_close = "1800s"
  }
}

# Alert policy for high memory usage
resource "google_monitoring_alert_policy" "high_memory" {
  display_name = "${var.app_name}-high-memory-usage"
  combiner     = "OR"

  conditions {
    display_name = "Memory usage > ${var.memory_threshold}%"

    condition_threshold {
      filter          = "resource.type = \"k8s_pod\" AND metric.type = \"kubernetes.io/memory/used_bytes\""
      duration        = "300s"
      comparison      = "COMPARISON_GT"
      threshold_value = var.memory_threshold / 100

      aggregations {
        alignment_period   = "60s"
        per_series_aligner = "ALIGN_MEAN"
      }
    }
  }

  notification_channels = var.notification_channels
  alert_strategy {
    auto_close = "1800s"
  }
}

# Alert policy for high CPU usage
resource "google_monitoring_alert_policy" "high_cpu" {
  display_name = "${var.app_name}-high-cpu-usage"
  combiner     = "OR"

  conditions {
    display_name = "CPU usage > ${var.cpu_threshold}%"

    condition_threshold {
      filter          = "resource.type = \"k8s_pod\" AND metric.type = \"kubernetes.io/cpu/core_usage_time\""
      duration        = "300s"
      comparison      = "COMPARISON_GT"
      threshold_value = var.cpu_threshold / 100

      aggregations {
        alignment_period   = "60s"
        per_series_aligner = "ALIGN_RATE"
      }
    }
  }

  notification_channels = var.notification_channels
  alert_strategy {
    auto_close = "1800s"
  }
}

# Alert policy for error rate
resource "google_monitoring_alert_policy" "error_rate" {
  display_name = "${var.app_name}-high-error-rate"
  combiner     = "OR"

  conditions {
    display_name = "Error rate > ${var.error_rate_threshold}%"

    condition_threshold {
      filter          = "resource.type = \"k8s_pod\" AND metric.type = \"custom.googleapis.com/http/requests_total\" AND metric.label.status =~ '^5.*'"
      duration        = "300s"
      comparison      = "COMPARISON_GT"
      threshold_value = var.error_rate_threshold / 100

      aggregations {
        alignment_period   = "60s"
        per_series_aligner = "ALIGN_RATE"
      }
    }
  }

  notification_channels = var.notification_channels
  alert_strategy {
    auto_close = "1800s"
  }
}

# Notification channel for email
resource "google_monitoring_notification_channel" "email" {
  count        = var.enable_email_notifications ? 1 : 0
  display_name = "Email: ${var.alert_email}"
  type         = "email"
  enabled      = true

  labels = {
    email_address = var.alert_email
  }
}

# Notification channel for Slack
resource "google_monitoring_notification_channel" "slack" {
  count        = var.enable_slack_notifications && var.slack_webhook_url != "" ? 1 : 0
  display_name = "Slack: ${var.slack_channel}"
  type         = "slack"
  enabled      = true

  labels = {
    channel_name = var.slack_channel
  }

  user_labels = {
    severity = "high"
  }

  sensitive_labels {
    auth_token = var.slack_webhook_url
  }
}

# Dashboard for cluster monitoring
resource "google_monitoring_dashboard" "cluster" {
  dashboard_json = jsonencode({
    displayName = "${var.app_name}-cluster-dashboard"
    mosaicLayout = {
      columns = 12
      tiles = [
        {
          width  = 6
          height = 4
          widget = {
            title = "CPU Usage by Pod"
            xyChart = {
              dataSets = [{
                timeSeriesQuery = {
                  timeSeriesFilter = {
                    filter = "resource.type = \"k8s_pod\" AND metric.type = \"kubernetes.io/cpu/core_usage_time\""
                  }
                }
              }]
            }
          }
        },
        {
          xPos   = 6
          width  = 6
          height = 4
          widget = {
            title = "Memory Usage by Pod"
            xyChart = {
              dataSets = [{
                timeSeriesQuery = {
                  timeSeriesFilter = {
                    filter = "resource.type = \"k8s_pod\" AND metric.type = \"kubernetes.io/memory/used_bytes\""
                  }
                }
              }]
            }
          }
        },
        {
          yPos   = 4
          width  = 6
          height = 4
          widget = {
            title = "Network In"
            xyChart = {
              dataSets = [{
                timeSeriesQuery = {
                  timeSeriesFilter = {
                    filter = "resource.type = \"k8s_pod\" AND metric.type = \"kubernetes.io/network/received_bytes_total\""
                  }
                }
              }]
            }
          }
        },
        {
          xPos   = 6
          yPos   = 4
          width  = 6
          height = 4
          widget = {
            title = "Network Out"
            xyChart = {
              dataSets = [{
                timeSeriesQuery = {
                  timeSeriesFilter = {
                    filter = "resource.type = \"k8s_pod\" AND metric.type = \"kubernetes.io/network/transmitted_bytes_total\""
                  }
                }
              }]
            }
          }
        },
        {
          yPos   = 8
          width  = 12
          height = 4
          widget = {
            title = "Pod Status"
            scorecard = {
              timeSeriesQuery = {
                timeSeriesFilter = {
                  filter = "resource.type = \"k8s_pod\" AND metric.type = \"kubernetes.io/pod/uptime\""
                }
              }
            }
          }
        }
      ]
    }
  })
}
