from __future__ import annotations

import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

import check_iac


class TerraformPolicyTests(unittest.TestCase):
    def test_prod_requires_multi_region_and_protection(self) -> None:
        violations = check_iac.validate_terraform_env(
            "prod",
            {
                "enable_multi_region": False,
                "secondary_region": "",
                "cluster_deletion_protection": False,
                "database_deletion_protection": False,
                "database_availability_type": "ZONAL",
                "enable_network_policies": False,
                "enable_ssh_access": True,
                "create_service_account_key": True,
            },
        )

        self.assertGreaterEqual(len(violations), 8)
        self.assertTrue(
            any("multi-region failover" in violation for violation in violations)
        )
        self.assertTrue(
            any("deletion protection" in violation for violation in violations)
        )
        self.assertTrue(
            any("service account keys" in violation for violation in violations)
        )

    def test_prod_rejects_unsafe_iam_exception_roles(self) -> None:
        violations = check_iac.validate_terraform_env(
            "prod",
            {
                "enable_multi_region": True,
                "secondary_region": "us-east1",
                "cluster_deletion_protection": True,
                "database_deletion_protection": True,
                "database_availability_type": "REGIONAL",
                "enable_network_policies": True,
                "enable_ssh_access": False,
                "create_service_account_key": False,
                "security_allow_production_iam_exceptions": True,
                "security_production_iam_exception_justification": "SEC-1234 temporary access for incident replay until 2026-12-31",
                "security_additional_permissions": ["roles/iam.serviceAccountUser"],
            },
        )

        self.assertTrue(
            any("unsafe IAM roles" in violation for violation in violations)
        )

    def test_prod_requires_documented_pubsub_exception(self) -> None:
        violations = check_iac.validate_terraform_env(
            "prod",
            {
                "enable_multi_region": True,
                "secondary_region": "us-east1",
                "cluster_deletion_protection": True,
                "database_deletion_protection": True,
                "database_availability_type": "REGIONAL",
                "enable_network_policies": True,
                "enable_ssh_access": False,
                "create_service_account_key": False,
                "security_pubsub_role": "roles/pubsub.subscriber",
            },
        )

        self.assertTrue(
            any("security_pubsub_role" in violation for violation in violations)
        )


class ManifestPolicyTests(unittest.TestCase):
    def _secure_rollout(self, analysis_steps: int = 3) -> dict:
        steps = []
        for _ in range(analysis_steps):
            steps.append({"setWeight": 10})
            steps.append(
                {
                    "analysis": {
                        "templates": [
                            {"templateName": check_iac.ERROR_BUDGET_TEMPLATE},
                            {"templateName": check_iac.QUEUE_RESILIENCE_TEMPLATE},
                        ],
                        "args": [
                            {
                                "name": "prometheus-address",
                                "value": "http://prometheus.monitoring.svc.cluster.local:9090",
                            }
                        ],
                    }
                }
            )

        return {
            "apiVersion": "argoproj.io/v1alpha1",
            "kind": "Rollout",
            "metadata": {"name": check_iac.ROLLOUT_NAME},
            "spec": {
                "strategy": {"canary": {"steps": steps}},
                "template": {
                    "spec": {
                        "securityContext": {
                            "seccompProfile": {"type": "RuntimeDefault"}
                        },
                        "containers": [
                            {
                                "name": "ingestion-gateway",
                                "readinessProbe": {"httpGet": {"path": "/readyz"}},
                                "livenessProbe": {"httpGet": {"path": "/healthz"}},
                                "resources": {
                                    "requests": {"cpu": "100m", "memory": "128Mi"},
                                    "limits": {"cpu": "500m", "memory": "512Mi"},
                                },
                                "securityContext": {
                                    "allowPrivilegeEscalation": False,
                                    "readOnlyRootFilesystem": True,
                                    "runAsNonRoot": True,
                                    "capabilities": {"drop": ["ALL"]},
                                },
                            }
                        ],
                    }
                },
            },
        }

    def _analysis_templates(self) -> list[dict]:
        return [
            {
                "kind": "AnalysisTemplate",
                "metadata": {"name": check_iac.ERROR_BUDGET_TEMPLATE},
            },
            {
                "kind": "AnalysisTemplate",
                "metadata": {"name": check_iac.QUEUE_RESILIENCE_TEMPLATE},
            },
        ]

    def _rollout_hpa(self, kind: str = "Rollout") -> dict:
        return {
            "kind": "HorizontalPodAutoscaler",
            "metadata": {"name": check_iac.HPA_NAME},
            "spec": {
                "scaleTargetRef": {
                    "apiVersion": "argoproj.io/v1alpha1",
                    "kind": kind,
                    "name": check_iac.ROLLOUT_NAME,
                }
            },
        }

    def test_prod_manifest_passes_when_secure(self) -> None:
        documents = self._analysis_templates() + [
            self._secure_rollout(),
            self._rollout_hpa(),
        ]

        violations = check_iac.validate_manifest_env("prod", documents)

        self.assertEqual([], violations)

    def test_prod_manifest_requires_enough_analysis_steps(self) -> None:
        documents = self._analysis_templates() + [
            self._secure_rollout(analysis_steps=2),
            self._rollout_hpa(),
        ]

        violations = check_iac.validate_manifest_env("prod", documents)

        self.assertTrue(
            any("at least 3 canary analysis steps" in violation for violation in violations)
        )

    def test_manifest_rejects_hpa_targeting_deployment(self) -> None:
        documents = self._analysis_templates() + [
            self._secure_rollout(),
            self._rollout_hpa(kind="Deployment"),
        ]

        violations = check_iac.validate_manifest_env("prod", documents)

        self.assertTrue(
            any("scaleTargetRef.kind must be Rollout" in violation for violation in violations)
        )

    def test_manifest_rejects_missing_container_hardening(self) -> None:
        rollout = self._secure_rollout()
        container = rollout["spec"]["template"]["spec"]["containers"][0]
        del container["readinessProbe"]
        container["securityContext"]["readOnlyRootFilesystem"] = False

        documents = self._analysis_templates() + [rollout, self._rollout_hpa()]

        violations = check_iac.validate_manifest_env("prod", documents)

        self.assertTrue(any("readinessProbe" in violation for violation in violations))
        self.assertTrue(
            any("readOnlyRootFilesystem=true" in violation for violation in violations)
        )


if __name__ == "__main__":
    unittest.main()
