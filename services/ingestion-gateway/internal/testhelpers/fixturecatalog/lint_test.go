package fixturecatalog

import "testing"

func TestCatalogLint_CurrentFixturesPass(t *testing.T) {
	catalog, err := Load()
	if err != nil {
		t.Fatalf("load fixture catalog: %v", err)
	}

	issues := catalog.Lint()
	if len(issues) != 0 {
		t.Fatalf("expected no lint issues, got %v", issues)
	}
}

func TestCatalogLint_RejectsMalformedFixtureConventions(t *testing.T) {
	catalog := Catalog{
		Fixtures: []Fixture{
			{
				ID:             "GitHub.PullRequest.baseline",
				Version:        1,
				Provider:       "GitHub",
				EventFamily:    "PullRequest",
				Variant:        "baseline",
				Path:           "bad path.json",
				Negative:       true,
				ExpectedStatus: 202,
				Publish:        true,
			},
		},
	}

	issues := catalog.Lint()
	if len(issues) < 5 {
		t.Fatalf("expected multiple lint issues, got %v", issues)
	}
}
