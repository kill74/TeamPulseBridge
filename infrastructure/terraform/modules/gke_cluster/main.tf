terraform {
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 5.0"
    }
  }
}

# Primary GKE cluster with production-grade configuration
resource "google_container_cluster" "primary" {
  name               = var.cluster_name
  location           = var.region
  initial_node_count = 1

  # Network configuration
  network    = var.network_name
  subnetwork = var.subnetwork_name

  # Cluster configuration
  cluster_autoscaling {
    enabled = true
    resource_limits {
      resource_type = "cpu"
      minimum       = var.cluster_min_cpu
      maximum       = var.cluster_max_cpu
    }
    resource_limits {
      resource_type = "memory"
      minimum       = var.cluster_min_memory
      maximum       = var.cluster_max_memory
    }
  }

  # Node pool
  node_pool {
    name       = "${var.cluster_name}-default-pool"
    node_count = var.initial_node_count

    autoscaling {
      min_node_count = var.min_node_count
      max_node_count = var.max_node_count
    }

    node_config {
      preemptible  = var.preemptible_nodes
      machine_type = var.machine_type
      disk_size_gb = var.disk_size_gb
      disk_type    = "pd-standard"

      oauth_scopes = [
        "https://www.googleapis.com/auth/cloud-platform",
        "https://www.googleapis.com/auth/compute",
        "https://www.googleapis.com/auth/devstorage.read_only",
        "https://www.googleapis.com/auth/logging.write",
        "https://www.googleapis.com/auth/monitoring",
        "https://www.googleapis.com/auth/service.management.readonly",
        "https://www.googleapis.com/auth/servicecontrol",
      ]

      workload_metadata_config {
        mode = "GKE_METADATA"
      }

      # Security context
      shielded_instance_config {
        enable_secure_boot          = true
        enable_integrity_monitoring = true
      }

      tags = concat(var.network_tags, ["gke-node", "${var.cluster_name}-node"])
      labels = merge(
        var.node_labels,
        {
          "env"     = var.environment
          "cluster" = var.cluster_name
        }
      )
    }

    management {
      auto_repair  = true
      auto_upgrade = true
    }
  }

  # Addons configuration
  addons_config {
    http_load_balancing {
      disabled = false
    }
    horizontal_pod_autoscaling {
      disabled = false
    }
    network_policy_config {
      disabled = false
    }
    cloudrun_config {
      disabled = true
    }
  }

  # IP allocation policy for VPC-native cluster
  ip_allocation_policy {
    cluster_secondary_range_name  = var.pods_secondary_range_name
    services_secondary_range_name = var.services_secondary_range_name
  }

  # Maintenance window
  maintenance_policy {
    daily_maintenance_window {
      start_time = "03:00"
    }
  }

  # Security settings
  master_auth {
    client_certificate_config {
      issue_client_certificate = false
    }
  }

  # Networking policy
  network_policy {
    enabled = true
  }

  # Resource labels
  resource_labels = merge(
    var.labels,
    {
      "environment" = var.environment
      "managed-by"  = "terraform"
    }
  )

  # Deletion protection
  deletion_protection = var.deletion_protection

  depends_on = [var.network_dependency]

  lifecycle {
    ignore_changes = [
      initial_node_count,
      node_pool,
    ]
  }
}

# Secondary node pool for workloads
resource "google_container_node_pool" "workload_pool" {
  name       = "${var.cluster_name}-workload-pool"
  location   = var.region
  cluster    = google_container_cluster.primary.name
  node_count = var.workload_node_count

  autoscaling {
    min_node_count = var.workload_min_node_count
    max_node_count = var.workload_max_node_count
  }

  management {
    auto_repair  = true
    auto_upgrade = true
  }

  node_config {
    preemptible  = var.workload_preemptible
    machine_type = var.workload_machine_type
    disk_size_gb = var.workload_disk_size_gb

    oauth_scopes = [
      "https://www.googleapis.com/auth/cloud-platform",
      "https://www.googleapis.com/auth/compute",
      "https://www.googleapis.com/auth/devstorage.read_only",
      "https://www.googleapis.com/auth/logging.write",
      "https://www.googleapis.com/auth/monitoring",
    ]

    workload_metadata_config {
      mode = "GKE_METADATA"
    }

    shielded_instance_config {
      enable_secure_boot          = true
      enable_integrity_monitoring = true
    }

    taint {
      key    = "workload"
      value  = "true"
      effect = "NO_SCHEDULE"
    }

    labels = merge(
      var.node_labels,
      {
        "env"  = var.environment
        "pool" = "workload"
      }
    )

    tags = concat(var.network_tags, ["gke-workload-node", "${var.cluster_name}-workload"])
  }
}

# Cluster role binding for metrics-server
resource "kubernetes_cluster_role_binding" "metrics_server" {
  count = var.manage_kubernetes_bootstrap_resources ? 1 : 0

  metadata {
    name = "metrics-server"
  }

  role_ref {
    api_group = "rbac.authorization.k8s.io"
    kind      = "ClusterRole"
    name      = "system:metrics-server"
  }

  subject {
    kind      = "ServiceAccount"
    name      = "metrics-server"
    namespace = "kube-system"
  }

  depends_on = [google_container_cluster.primary]
}

# Output kubeconfig for local access
resource "local_file" "kubeconfig" {
  count           = var.generate_local_kubeconfig ? 1 : 0
  filename        = "${path.module}/kubeconfig-${var.cluster_name}.yaml"
  content         = base64decode(google_container_cluster.primary.master_auth[0].cluster_ca_certificate)
  file_permission = "0600"
}
