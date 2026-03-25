#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<EOF
Usage: $0 <gcp-project-id> <gke-cluster-name> <gke-region> [repo-url] [git-revision]

Example:
  $0 my-project teampulse-staging us-central1 https://github.com/kill74/TeamPulseBridge.git main
EOF
}

if [[ ${1:-} == "-h" || ${1:-} == "--help" ]]; then
  usage
  exit 0
fi

if [[ $# -lt 3 ]]; then
  usage
  exit 1
fi

PROJECT_ID="$1"
CLUSTER_NAME="$2"
REGION="$3"
REPO_URL="${4:-https://github.com/kill74/TeamPulseBridge.git}"
GIT_REVISION="${5:-main}"

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
ARGO_DIR="$ROOT_DIR/deploy/gitops/argocd"

if ! command -v gcloud >/dev/null 2>&1; then
  echo "gcloud CLI is required"
  exit 1
fi

if ! command -v kubectl >/dev/null 2>&1; then
  echo "kubectl is required"
  exit 1
fi

wait_for_app() {
  local app_name="$1"
  local max_attempts="${2:-30}"
  local attempt=1

  while (( attempt <= max_attempts )); do
    if kubectl -n argocd get application "$app_name" >/dev/null 2>&1; then
      return 0
    fi
    sleep 2
    attempt=$((attempt + 1))
  done

  echo "Timed out waiting for Argo CD application: $app_name"
  return 1
}

echo "[1/5] Fetching GKE credentials"
gcloud container clusters get-credentials "$CLUSTER_NAME" --region "$REGION" --project "$PROJECT_ID"

echo "[2/5] Installing Argo CD"
kubectl create namespace argocd --dry-run=client -o yaml | kubectl apply -f -
kubectl apply -n argocd -f https://raw.githubusercontent.com/argoproj/argo-cd/v2.13.2/manifests/install.yaml

echo "[3/5] Waiting for Argo CD API server"
kubectl -n argocd rollout status deployment/argocd-server --timeout=300s

echo "[4/5] Applying TeamPulse Argo CD project + root app"
kubectl apply -k "$ARGO_DIR"

wait_for_app "teampulse-root"
wait_for_app "ingestion-gateway-staging"
wait_for_app "ingestion-gateway-prod"

echo "[5/5] Overriding repository source and revision"
kubectl -n argocd patch application teampulse-root --type merge -p "{\"spec\":{\"source\":{\"repoURL\":\"$REPO_URL\",\"targetRevision\":\"$GIT_REVISION\"}}}"
kubectl -n argocd patch application ingestion-gateway-staging --type merge -p "{\"spec\":{\"source\":{\"repoURL\":\"$REPO_URL\",\"targetRevision\":\"$GIT_REVISION\"}}}"
kubectl -n argocd patch application ingestion-gateway-prod --type merge -p "{\"spec\":{\"source\":{\"repoURL\":\"$REPO_URL\",\"targetRevision\":\"$GIT_REVISION\"}}}"

echo "GitOps bootstrap complete."
echo "Get initial admin password with: kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath='{.data.password}' | base64 -d; echo"
