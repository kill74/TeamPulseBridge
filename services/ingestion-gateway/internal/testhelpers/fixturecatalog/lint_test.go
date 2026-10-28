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

func TestCatalogCoverageSummary_ReportsProviderAndFamilyCoverage(t *testing.T) {
	catalog, err := Load()
	if err != nil {
		t.Fatalf("load fixture catalog: %v", err)
	}

	summary := catalog.CoverageSummary()
	if summary.TotalFixtures == 0 {
		t.Fatal("expected at least one fixture in coverage summary")
	}
	if summary.Providers == 0 {
		t.Fatal("expected at least one provider in coverage summary")
	}
	if summary.Families == 0 {
		t.Fatal("expected at least one event family in coverage summary")
	}
	if summary.Publishable == 0 {
		t.Fatal("expected at least one publishable fixture in coverage summary")
	}
	if _, ok := summary.ByProvider["github"]; !ok {
		t.Fatal("expected github provider coverage to be reported")
	}
	if summary.ByProvider["github"].Fixtures == 0 {
		t.Fatal("expected github fixtures to be counted")
	}
}
