terraform {
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 7.35"
    }
    google-beta = {
      source  = "hashicorp/google-beta"
      version = "~> 7.36"
    }
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = "~> 3.2"
    }
    local = {
      source  = "hashicorp/local"
      version = "~> 2.4"
    }
    random = {
      source  = "hashicorp/random"
      version = "~> 3.5"
    }
  }

  required_version = ">= 1.0"
}
