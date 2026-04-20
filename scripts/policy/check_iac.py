#!/usr/bin/env python3
from __future__ import annotations

import argparse
import sys
from pathlib import Path
from typing import Any

import hcl2
import yaml

ERROR_BUDGET_TEMPLATE = "ingestion-gateway-error-budget"
QUEUE_RESILIENCE_TEMPLATE = "ingestion-gateway-queue-resilience"
ROLLOUT_NAME = "ingestion-gateway"
HPA_NAME = "ingestion-gateway"


def parse_named_path(value: str) -> tuple[str, Path]:
    if "=" not in value:
        raise argparse.ArgumentTypeError(
            f"expected NAME=PATH format, received {value!r}"
        )

    name, raw_path = value.split("=", 1)
    name = name.strip()
    path = Path(raw_path.strip())

    if not name:
        raise argparse.ArgumentTypeError("policy input name cannot be empty")
    if not path:
        raise argparse.ArgumentTypeError("policy input path cannot be empty")

    return name, path


def load_tfvars(path: Path) -> dict[str, Any]:
    with path.open("r", encoding="utf-8") as handle:
        data = normalize_hcl_value(hcl2.load(handle))
    return data


def normalize_hcl_value(value: Any) -> Any:
    if isinstance(value, list):
        if len(value) == 1:
            return normalize_hcl_value(value[0])
        return [normalize_hcl_value(item) for item in value]
    if isinstance(value, dict):
        return {key: normalize_hcl_value(item) for key, item in value.items()}
    return value


def read_text_with_fallback(path: Path) -> str:
    last_error: UnicodeError | None = None
    for encoding in ("utf-8", "utf-8-sig", "utf-16", "utf-16-le", "utf-16-be"):
        try:
            return path.read_text(encoding=encoding)
        except UnicodeError as error:
            last_error = error

    if last_error is not None:
        raise last_error
    return path.read_text()


def load_manifests(path: Path) -> list[dict[str, Any]]:
    documents = [
        document
        for document in yaml.safe_load_all(read_text_with_fallback(path))
        if isinstance(document, dict)
    ]
    return documents


def validate_terraform_env(environment: str, values: dict[str, Any]) -> list[str]:
    violations: list[str] = []

    def require(condition: bool, message: str) -> None:
        if not condition:
            violations.append(f"[terraform:{environment}] {message}")

    if environment == "prod":
        require(
            values.get("enable_multi_region") is True,
            "production must enable multi-region failover",
        )
        require(
            bool(str(values.get("secondary_region", "")).strip()),
            "production must define a non-empty secondary_region",
        )
        require(
            values.get("cluster_deletion_protection") is True,
            "production clusters must keep deletion protection enabled",
        )
        require(
            values.get("database_deletion_protection") is True,
            "production databases must keep deletion protection enabled",
        )
        require(
            values.get("database_availability_type") == "REGIONAL",
            "production databases must use REGIONAL availability",
        )
        require(
            values.get("enable_network_policies") is True,
            "production must keep Kubernetes network policies enabled",
        )
        require(
            values.get("enable_ssh_access") is False,
            "production must not allow SSH access",
        )

    return violations


def find_documents(
    documents: list[dict[str, Any]], kind: str, name: str
) -> list[dict[str, Any]]:
    matches: list[dict[str, Any]] = []
    for document in documents:
        metadata = document.get("metadata") or {}
        if document.get("kind") == kind and metadata.get("name") == name:
            matches.append(document)
    return matches


def validate_container_security(
    environment: str, rollout_name: str, container: dict[str, Any]
) -> list[str]:
    violations: list[str] = []
    container_name = container.get("name", "<unnamed>")

    def require(condition: bool, message: str) -> None:
        if not condition:
            violations.append(
                f"[kubernetes:{environment}] rollout/{rollout_name} container/{container_name}: {message}"
            )

    resources = container.get("resources") or {}
    requests = resources.get("requests") or {}
    limits = resources.get("limits") or {}
    security_context = container.get("securityContext") or {}
    capabilities = security_context.get("capabilities") or {}
    dropped_capabilities = capabilities.get("drop") or []

    require("readinessProbe" in container, "must define a readinessProbe")
    require("livenessProbe" in container, "must define a livenessProbe")
    require(bool(requests.get("cpu")), "must define cpu requests")
    require(bool(requests.get("memory")), "must define memory requests")
    require(bool(limits.get("cpu")), "must define cpu limits")
    require(bool(limits.get("memory")), "must define memory limits")
    require(
        security_context.get("allowPrivilegeEscalation") is False,
        "must set allowPrivilegeEscalation=false",
    )
    require(
        security_context.get("readOnlyRootFilesystem") is True,
        "must set readOnlyRootFilesystem=true",
    )
    require(
        security_context.get("runAsNonRoot") is True,
        "must set runAsNonRoot=true",
    )
    require(
        "ALL" in dropped_capabilities,
        "must drop ALL Linux capabilities",
    )

    return violations


def validate_rollout(environment: str, rollout: dict[str, Any]) -> list[str]:
    violations: list[str] = []
    rollout_name = rollout.get("metadata", {}).get("name", "<unknown>")
    spec = rollout.get("spec") or {}
    canary = (spec.get("strategy") or {}).get("canary") or {}
    steps = canary.get("steps") or []
    analysis_steps = [
        step.get("analysis")
        for step in steps
        if isinstance(step, dict) and isinstance(step.get("analysis"), dict)
    ]
    minimum_analysis_steps = {"staging": 2, "prod": 3}.get(environment, 1)

    def require(condition: bool, message: str) -> None:
        if not condition:
            violations.append(f"[kubernetes:{environment}] rollout/{rollout_name}: {message}")

    require(bool(steps), "must define canary steps")
    require(
        len(analysis_steps) >= minimum_analysis_steps,
        f"must define at least {minimum_analysis_steps} canary analysis steps",
    )

    for index, analysis in enumerate(analysis_steps, start=1):
        templates = {
            template.get("templateName")
            for template in analysis.get("templates", [])
            if isinstance(template, dict)
        }
        required_templates = {ERROR_BUDGET_TEMPLATE, QUEUE_RESILIENCE_TEMPLATE}
        missing_templates = sorted(required_templates - templates)
        if missing_templates:
            require(
                False,
                f"analysis step {index} must include templates {', '.join(missing_templates)}",
            )

        prometheus_args = [
            arg
            for arg in analysis.get("args", [])
            if isinstance(arg, dict) and arg.get("name") == "prometheus-address"
        ]
        require(
            any(str(arg.get("value", "")).strip() for arg in prometheus_args),
            f"analysis step {index} must define a non-empty prometheus-address arg",
        )

    template_spec = ((spec.get("template") or {}).get("spec") or {})
    pod_security_context = template_spec.get("securityContext") or {}
    seccomp_profile = pod_security_context.get("seccompProfile") or {}
    require(
        seccomp_profile.get("type") == "RuntimeDefault",
        "must set pod seccompProfile.type=RuntimeDefault",
    )

    containers = template_spec.get("containers") or []
    require(bool(containers), "must define at least one application container")
    for container in containers:
        violations.extend(validate_container_security(environment, rollout_name, container))

    return violations


def validate_hpa(environment: str, hpa: dict[str, Any]) -> list[str]:
    name = hpa.get("metadata", {}).get("name", "<unknown>")
    scale_target_ref = (hpa.get("spec") or {}).get("scaleTargetRef") or {}
    violations: list[str] = []

    if scale_target_ref.get("kind") != "Rollout":
        violations.append(
            f"[kubernetes:{environment}] hpa/{name}: scaleTargetRef.kind must be Rollout"
        )
    if scale_target_ref.get("apiVersion") != "argoproj.io/v1alpha1":
        violations.append(
            f"[kubernetes:{environment}] hpa/{name}: scaleTargetRef.apiVersion must be argoproj.io/v1alpha1"
        )

    return violations


def validate_manifest_env(
    environment: str, documents: list[dict[str, Any]]
) -> list[str]:
    violations: list[str] = []
    analysis_template_names = {
        document.get("metadata", {}).get("name")
        for document in documents
        if document.get("kind") == "AnalysisTemplate"
    }

    rollout_matches = find_documents(documents, "Rollout", ROLLOUT_NAME)
    hpa_matches = find_documents(documents, "HorizontalPodAutoscaler", HPA_NAME)

    if len(rollout_matches) != 1:
        violations.append(
            f"[kubernetes:{environment}] expected exactly one rollout/{ROLLOUT_NAME}, found {len(rollout_matches)}"
        )
    if len(hpa_matches) != 1:
        violations.append(
            f"[kubernetes:{environment}] expected exactly one hpa/{HPA_NAME}, found {len(hpa_matches)}"
        )

    for template_name in (ERROR_BUDGET_TEMPLATE, QUEUE_RESILIENCE_TEMPLATE):
        if template_name not in analysis_template_names:
            violations.append(
                f"[kubernetes:{environment}] missing analysis template {template_name}"
            )

    if rollout_matches:
        violations.extend(validate_rollout(environment, rollout_matches[0]))
    if hpa_matches:
        violations.extend(validate_hpa(environment, hpa_matches[0]))

    return violations


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        description="Validate repo-specific Terraform and Kubernetes policies."
    )
    parser.add_argument(
        "--terraform-env",
        action="append",
        default=[],
        type=parse_named_path,
        help="Terraform environment in NAME=PATH format. May be supplied multiple times.",
    )
    parser.add_argument(
        "--manifest-env",
        action="append",
        default=[],
        type=parse_named_path,
        help="Rendered manifest bundle in NAME=PATH format. May be supplied multiple times.",
    )
    return parser


def main(argv: list[str] | None = None) -> int:
    parser = build_parser()
    args = parser.parse_args(argv)

    violations: list[str] = []

    for environment, path in args.terraform_env:
        if not path.exists():
            violations.append(f"[terraform:{environment}] missing file: {path}")
            continue
        violations.extend(validate_terraform_env(environment, load_tfvars(path)))

    for environment, path in args.manifest_env:
        if not path.exists():
            violations.append(f"[kubernetes:{environment}] missing file: {path}")
            continue
        violations.extend(validate_manifest_env(environment, load_manifests(path)))

    if violations:
        print("Policy violations detected:", file=sys.stderr)
        for violation in violations:
            print(f" - {violation}", file=sys.stderr)
        return 1

    print("Repo-specific IaC policies passed.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
