terraform {
  backend "gcs" {
    # Keep backend values out of VCS and pass them via:
    # - terraform init -backend-config=environments/<env>/backend.conf
    # - CI/CD secret variables (preferred for production)
    #
    # Expected backend.conf keys:
    # bucket = "<terraform-state-bucket>"
    # prefix = "teampulse/<environment>"
    #
    # Optional key:
    # encryption_key = "<base64-encoded-cmek>"
  }
}

# Example backend.conf for staging:
# bucket = "my-org-terraform-state"
# prefix = "teampulse/staging"
# encryption_key = "<staging-cmek-base64>"

# Example backend.conf for prod:
# bucket = "my-org-terraform-state"
# prefix = "teampulse/prod"
# encryption_key = "<prod-cmek-base64>"
