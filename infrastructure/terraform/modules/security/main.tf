terraform {
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 5.0"
    }
  }
}

locals {
  production_iam_exception_expiry_matches = regexall("20[0-9]{2}-[01][0-9]-[0-3][0-9]", var.production_iam_exception_justification)
  production_iam_exception_expiry         = length(local.production_iam_exception_expiry_matches) > 0 ? local.production_iam_exception_expiry_matches[0] : ""
}

# Service account for GKE nodes
resource "google_service_account" "gke_nodes" {
  account_id   = "${var.cluster_name}-nodes"
  display_name = "GKE nodes service account for ${var.environment}"
  description  = "Service account for GKE cluster nodes"
}

# Service account for application workloads
resource "google_service_account" "app_workload" {
  account_id   = "${var.app_name}-workload"
  display_name = "${var.app_name} workload service account"
  description  = "Service account for ${var.app_name} application running in GKE"
}

# Service account for database access
resource "google_service_account" "db_access" {
  account_id   = "${var.app_name}-db"
  display_name = "${var.app_name} database access account"
  description  = "Service account for database access via IAM authentication"
}

# Service account for Pub/Sub operations
resource "google_service_account" "pubsub" {
  account_id   = "${var.app_name}-pubsub"
  display_name = "${var.app_name} Pub/Sub service account"
  description  = "Service account for Google Pub/Sub operations"
}

# IAM Binding for application to use database via Workload Identity
resource "google_service_account_iam_member" "workload_identity_user" {
  service_account_id = google_service_account.app_workload.name
  role               = "roles/iam.workloadIdentityUser"
  member             = "serviceAccount:${var.gcp_project}.svc.id.goog[${var.namespace}/${var.ksa_name}]"
}

# IAM Binding for database access
resource "google_project_iam_member" "cloudsql_client" {
  project = var.gcp_project
  role    = "roles/cloudsql.client"
  member  = "serviceAccount:${google_service_account.app_workload.email}"
}

# Pub/Sub role for application (least privilege by default)
resource "google_project_iam_member" "pubsub_access" {
  project = var.gcp_project
  role    = var.pubsub_role
  member  = "serviceAccount:${google_service_account.app_workload.email}"
}

# Optional additional project roles for workload SA
resource "google_project_iam_member" "additional_permissions" {
  for_each = toset(var.permissions)

  project = var.gcp_project
  role    = each.value
  member  = "serviceAccount:${google_service_account.app_workload.email}"

  dynamic "condition" {
    for_each = var.environment == "prod" && local.production_iam_exception_expiry != "" ? [local.production_iam_exception_expiry] : []
    content {
      title       = "temporary_prod_exception"
      description = "Temporary production IAM exception expiring on ${condition.value}"
      expression  = "request.time < timestamp(\"${condition.value}T23:59:59Z\")"
    }
  }
}

# Logging roles
resource "google_project_iam_member" "logging_log_writer" {
  project = var.gcp_project
  role    = "roles/logging.logWriter"
  member  = "serviceAccount:${google_service_account.app_workload.email}"
}

# Monitoring roles
resource "google_project_iam_member" "monitoring_metric_writer" {
  project = var.gcp_project
  role    = "roles/monitoring.metricWriter"
  member  = "serviceAccount:${google_service_account.app_workload.email}"
}

# Tracing roles
resource "google_project_iam_member" "cloudtrace_agent" {
  project = var.gcp_project
  role    = "roles/cloudtrace.agent"
  member  = "serviceAccount:${google_service_account.app_workload.email}"
}

# Kubernetes RBAC ClusterRole for reading metrics
resource "kubernetes_cluster_role" "app_metrics_reader" {
  count = var.manage_kubernetes_security_resources ? 1 : 0

  metadata {
    name = "${var.app_name}-metrics-reader"
  }

  rule {
    api_groups = [""]
    resources  = ["pods"]
    verbs      = ["get", "list", "watch"]
  }

  rule {
    api_groups = [""]
    resources  = ["nodes"]
    verbs      = ["get", "list"]
  }
}

# Kubernetes RBAC ClusterRoleBinding
resource "kubernetes_cluster_role_binding" "app_metrics_reader" {
  count = var.manage_kubernetes_security_resources ? 1 : 0

  metadata {
    name = "${var.app_name}-metrics-reader"
  }

  role_ref {
    api_group = "rbac.authorization.k8s.io"
    kind      = "ClusterRole"
    name      = kubernetes_cluster_role.app_metrics_reader[0].metadata[0].name
  }

  subject {
    kind      = "ServiceAccount"
    name      = var.ksa_name
    namespace = var.namespace
  }

  depends_on = [kubernetes_cluster_role.app_metrics_reader]
}

# NetworkPolicy to restrict ingress traffic
resource "kubernetes_network_policy" "app_ingress" {
  count = var.manage_kubernetes_security_resources && var.enable_network_policies ? 1 : 0

  metadata {
    name      = "${var.app_name}-ingress"
    namespace = var.namespace
  }

  spec {
    pod_selector {
      match_labels = {
        app = var.app_name
      }
    }

    policy_types = ["Ingress"]

    ingress {
      from {
        pod_selector {
          match_labels = {
            role = "frontend"
          }
        }
      }

      from {
        namespace_selector {
          match_labels = {
            name = "ingress-nginx"
          }
        }
      }

      ports {
        protocol = "TCP"
        port     = "8080"
      }
    }
  }
}

# NetworkPolicy to restrict egress traffic
resource "kubernetes_network_policy" "app_egress" {
  count = var.manage_kubernetes_security_resources && var.enable_network_policies ? 1 : 0

  metadata {
    name      = "${var.app_name}-egress"
    namespace = var.namespace
  }

  spec {
    pod_selector {
      match_labels = {
        app = var.app_name
      }
    }

    policy_types = ["Egress"]

    egress {
      to {
        namespace_selector {
          match_labels = {
            name = "kube-system"
          }
        }
      }
    }

    egress {
      to {
        pod_selector {
          match_labels = {
            role = "backend"
          }
        }
      }
    }

    egress {
      dynamic "to" {
        for_each = var.https_egress_cidrs
        content {
          ip_block {
            cidr = to.value
          }
        }
      }

      ports {
        protocol = "TCP"
        port     = "443"
      }
    }

    egress {
      dynamic "to" {
        for_each = var.db_egress_cidrs
        content {
          ip_block {
            cidr = to.value
          }
        }
      }

      ports {
        protocol = "TCP"
        port     = "5432"
      }
    }
  }
}

# Pod Security Policy
resource "kubernetes_pod_security_policy" "restricted" {
  count = var.manage_kubernetes_security_resources && var.enable_pod_security_policy ? 1 : 0

  metadata {
    name = "${var.app_name}-restricted-psp"
  }

  spec {
    privileged                 = false
    allow_privilege_escalation = false

    required_drop_capabilities = [
      "ALL"
    ]

    allowed_capabilities = [
      "NET_BIND_SERVICE"
    ]

    volumes = [
      "configMap",
      "emptyDir",
      "projected",
      "secret",
      "downwardAPI",
      "persistentVolumeClaim"
    ]

    host_network = false
    host_ipc     = false
    host_pid     = false

    run_as_user {
      rule = "MustRunAsNonRoot"
    }

    se_linux {
      rule = "MustRunAs"

      se_linux_options {
        user  = "system_u"
        role  = "system_r"
        type  = "container_t"
        level = "s0:c123,c456"
      }
    }

    fs_group {
      rule = "MustRunAs"

      range {
        min = 1000
        max = 65535
      }
    }

    supplemental_groups {
      rule = "MustRunAs"

      range {
        min = 1000
        max = 65535
      }
    }

    read_only_root_filesystem = true
  }
}

# ClusterRoleBinding for PSP
resource "kubernetes_cluster_role_binding" "psp_restricted" {
  count = var.manage_kubernetes_security_resources && var.enable_pod_security_policy ? 1 : 0

  metadata {
    name = "${var.app_name}-psp-all-serviceaccounts"
  }

  role_ref {
    api_group = "rbac.authorization.k8s.io"
    kind      = "ClusterRole"
    name      = "edit"
  }

  subject {
    kind      = "Group"
    name      = "system:serviceaccounts"
    api_group = "rbac.authorization.k8s.io"
  }

  depends_on = [kubernetes_pod_security_policy.restricted]
}

# Service account key for application
resource "google_service_account_key" "app_key" {
  count              = var.create_service_account_key ? 1 : 0
  service_account_id = google_service_account.app_workload.name
  public_key_type    = "TYPE_X509_PEM_FILE"
}
