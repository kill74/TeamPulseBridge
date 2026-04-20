output "vpc_network_id" {
  description = "VPC network ID"
  value       = google_compute_network.vpc.id
}

output "vpc_network_name" {
  description = "VPC network name"
  value       = google_compute_network.vpc.name
}

output "vpc_network_self_link" {
  description = "VPC network self link"
  value       = google_compute_network.vpc.self_link
}

output "gke_subnet_id" {
  description = "GKE subnet ID"
  value       = google_compute_subnetwork.gke_subnet.id
}

output "gke_subnet_name" {
  description = "GKE subnet name"
  value       = google_compute_subnetwork.gke_subnet.name
}

output "gke_subnet_self_link" {
  description = "GKE subnet self link"
  value       = google_compute_subnetwork.gke_subnet.self_link
}

output "db_subnet_id" {
  description = "Database subnet ID"
  value       = try(google_compute_subnetwork.db_subnet[0].id, null)
}

output "db_subnet_name" {
  description = "Database subnet name"
  value       = try(google_compute_subnetwork.db_subnet[0].name, null)
}

output "pods_secondary_range_name" {
  description = "Secondary range name for pods"
  value       = "pods"
}

output "services_secondary_range_name" {
  description = "Secondary range name for services"
  value       = "services"
}

output "security_policy_id" {
  description = "Cloud Armor security policy ID"
  value       = google_compute_security_policy.policy.id
}

output "nat_gateway_ip" {
  description = "NAT gateway external IP (for allowlist)"
  value       = try(coalescelist(google_compute_router_nat.nat.nat_ips, []), [])
}
