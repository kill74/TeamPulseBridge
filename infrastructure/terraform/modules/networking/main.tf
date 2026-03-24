terraform {
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 5.0"
    }
  }
}

# VPC Network
resource "google_compute_network" "vpc" {
  name                    = var.network_name
  auto_create_subnetworks = false
  routing_mode            = "REGIONAL"

  description = "VPC network for ${var.environment} environment"
}

# Primary subnet for GKE cluster
resource "google_compute_subnetwork" "gke_subnet" {
  name          = "${var.network_name}-gke-subnet"
  ip_cidr_range = var.gke_subnet_cidr
  network       = google_compute_network.vpc.id
  region        = var.region

  description = "Subnet for GKE cluster nodes"

  secondary_ip_range {
    range_name    = "pods"
    ip_cidr_range = var.pods_cidr
  }

  secondary_ip_range {
    range_name    = "services"
    ip_cidr_range = var.services_cidr
  }

  private_ip_google_access = true

  log_config {
    aggregation_interval = "INTERVAL_5_SEC"
    flow_sampling        = 0.5
    metadata             = "INCLUDE_ALL_METADATA"
  }
}

# Database subnet for Cloud SQL
resource "google_compute_subnetwork" "db_subnet" {
  count         = var.create_db_subnet ? 1 : 0
  name          = "${var.network_name}-db-subnet"
  ip_cidr_range = var.db_subnet_cidr
  network       = google_compute_network.vpc.id
  region        = var.region

  description           = "Subnet for database resources"
  private_ip_google_access = true
}

# External NAT for outbound traffic
resource "google_compute_router" "router" {
  name    = "${var.network_name}-router"
  region  = var.region
  network = google_compute_network.vpc.id

  bgp {
    asn = 64514
  }
}

resource "google_compute_router_nat" "nat" {
  name                               = "${var.network_name}-nat"
  router                             = google_compute_router.router.name
  region                             = google_compute_router.router.region
  nat_ip_allocate_option             = "AUTO_ONLY"
  source_subnetwork_ip_ranges_to_nat = "ALL_SUBNETWORKS_ALL_IP_RANGES"

  log_config {
    enable = true
    filter = "ERRORS_ONLY"
  }
}

# Allow internal traffic (VPC peering)
resource "google_compute_firewall" "allow_internal" {
  name      = "${var.network_name}-allow-internal"
  network   = google_compute_network.vpc.id
  direction = "INGRESS"

  allow {
    protocol = "tcp"
  }

  allow {
    protocol = "udp"
  }

  allow {
    protocol = "icmp"
  }

  source_ranges = [
    var.gke_subnet_cidr,
    var.pods_cidr,
    var.services_cidr,
  ]

  target_tags = ["gke-node"]

  description = "Allow internal VPC traffic"
}

# Allow SSH for debugging (restricted)
resource "google_compute_firewall" "allow_ssh" {
  count     = var.enable_ssh_access ? 1 : 0
  name      = "${var.network_name}-allow-ssh"
  network   = google_compute_network.vpc.id
  direction = "INGRESS"
  priority  = 1000

  allow {
    protocol = "tcp"
    ports    = ["22"]
  }

  source_ranges = var.ssh_source_ranges
  target_tags   = ["gke-node"]

  description = "Allow SSH access from specified IPs"
}

# Allow health checks
resource "google_compute_firewall" "allow_health_checks" {
  name      = "${var.network_name}-allow-health-checks"
  network   = google_compute_network.vpc.id
  direction = "INGRESS"
  priority  = 1000

  allow {
    protocol = "tcp"
    ports    = ["8080", "9090"]
  }

  source_ranges = [
    "35.191.0.0/16",
    "130.211.0.0/22",
  ]

  target_tags = ["gke-node"]

  description = "Allow GCP health checks"
}

# Allow inbound HTTP/HTTPS
resource "google_compute_firewall" "allow_http_https" {
  name      = "${var.network_name}-allow-http-https"
  network   = google_compute_network.vpc.id
  direction = "INGRESS"
  priority  = 1000

  allow {
    protocol = "tcp"
    ports    = ["80", "443"]
  }

  source_ranges = ["0.0.0.0/0"]
  target_tags   = ["gke-node"]

  description = "Allow incoming HTTP/HTTPS traffic"
}

# Private Service Connection for Cloud SQL
resource "google_compute_global_address" "private_ip_address" {
  count         = var.create_db_subnet ? 1 : 0
  name          = "${var.network_name}-private-ip"
  purpose       = "VPC_PEERING"
  address_type  = "INTERNAL"
  prefix_length = 16
  network       = google_compute_network.vpc.id
}

resource "google_service_networking_connection" "private_vpc_connection" {
  count                   = var.create_db_subnet ? 1 : 0
  network                 = google_compute_network.vpc.id
  service                 = "servicenetworking.googleapis.com"
  reserved_peering_ranges = [google_compute_global_address.private_ip_address[0].name]
}

# Cloud Armor Security Policy for DDoS protection
resource "google_compute_security_policy" "policy" {
  name        = "${var.network_name}-security-policy"
  description = "Security policy with DDoS protection and rate limiting"

  # Default rule
  rules {
    action   = "allow"
    priority = "2147483647"
    match {
      versioned_expr = "EXPR_V2"
      expr {
        expression = "true"
      }
    }
    description = "Default rule"
  }

  # Rate limiting rule
  rules {
    action   = "rate_based_ban"
    priority = "1001"
    match {
      versioned_expr = "EXPR_V2"
      expr {
        expression = "true"
      }
    }
    rate_limit_options {
      conform_action = "allow"
      exceed_action  = "deny(429)"

      rate_limit_threshold {
        count        = 100
        interval_sec = 60
      }

      ban_duration_sec = 600
    }
    description = "Rate limit - 100 requests per minute"
  }

  # Block common attacks
  rules {
    action   = "deny(403)"
    priority = "1000"
    match {
      versioned_expr = "EXPR_V2"
      expr {
        expression = "evaluatePreconfiguredExpr('sqli-v33-stable', ['owasp-crs-v030001-id942251-sqli', 'owasp-crs-v030001-id942420-sqli', 'owasp-crs-v030001-id942431-sqli'])"
      }
    }
    description = "SQL injection protection"
  }
}
