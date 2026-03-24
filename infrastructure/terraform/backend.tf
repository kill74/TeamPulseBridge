terraform {
  backend "gcs" {
    # Configure these via -backend-config flag or backend.conf file:
    # bucket         = "your-terraform-state-bucket"
    # prefix         = "teampulse/[staging|prod]"
    # encryption_key = "your-base64-encoded-key"
  }
}

# Example backend.conf for staging:
# bucket         = "my-org-terraform-state"
# prefix         = "teampulse/staging"
# encryption_key = "CjD3EIsSNFvyL7vrr3c3MK7i/Yc/7utjzHeQYcKZ7Ew="

# Example backend.conf for prod:
# bucket         = "my-org-terraform-state"
# prefix         = "teampulse/prod"
# encryption_key = "different-base64-encoded-key"
