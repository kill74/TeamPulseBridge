package fixturecatalog

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var (
	fixtureSlugPattern = regexp.MustCompile(`^[a-z0-9]+(?:_[a-z0-9]+)*$`)
	fixturePathPattern = regexp.MustCompile(`^[a-z0-9]+(?:_[a-z0-9]+)*\.json$`)
)

func (c Catalog) Lint() []error {
	issues := make([]error, 0)
	baselinesByFamily := make(map[string]int, len(c.Fixtures))
	families := make(map[string]struct{}, len(c.Fixtures))

	for _, fixture := range c.Fixtures {
		provider := strings.TrimSpace(fixture.Provider)
		eventFamily := strings.TrimSpace(fixture.EventFamily)
		variant := strings.TrimSpace(fixture.Variant)
		path := strings.TrimSpace(fixture.Path)
		familyKey := provider + "|" + eventFamily
		families[familyKey] = struct{}{}

		if !fixtureSlugPattern.MatchString(provider) {
			issues = append(issues, fmt.Errorf("fixture %q provider must be lowercase snake_case, got %q", fixture.ID, provider))
		}
		if !fixtureSlugPattern.MatchString(eventFamily) {
			issues = append(issues, fmt.Errorf("fixture %q event_family must be lowercase snake_case, got %q", fixture.ID, eventFamily))
		}
		if !fixtureSlugPattern.MatchString(variant) {
			issues = append(issues, fmt.Errorf("fixture %q variant must be lowercase snake_case, got %q", fixture.ID, variant))
		}

		expectedID := fmt.Sprintf("%s.%s.%s.v%d", provider, eventFamily, variant, fixture.Version)
		if fixture.ID != expectedID {
			issues = append(issues, fmt.Errorf("fixture %q must follow id format %q", fixture.ID, expectedID))
		}

		baseName := filepath.Base(path)
		if baseName != path {
			issues = append(issues, fmt.Errorf("fixture %q path must be a file name without nested directories, got %q", fixture.ID, path))
		}
		if !fixturePathPattern.MatchString(baseName) {
			issues = append(issues, fmt.Errorf("fixture %q path must be lowercase snake_case ending in .json, got %q", fixture.ID, path))
		}
		if provider != "" && !strings.HasPrefix(baseName, provider+"_") {
			issues = append(issues, fmt.Errorf("fixture %q path %q must start with provider prefix %q", fixture.ID, path, provider+"_"))
		}
		if strings.TrimSpace(fixture.Notes) == "" {
			issues = append(issues, fmt.Errorf("fixture %q notes must not be empty", fixture.ID))
		}

		if variant == "baseline" {
			baselinesByFamily[familyKey]++
			if fixture.Negative {
				issues = append(issues, fmt.Errorf("fixture %q baseline variant must not be marked negative", fixture.ID))
			}
		}
		if fixture.Publish && (fixture.ExpectedStatus < 200 || fixture.ExpectedStatus >= 300) {
			issues = append(issues, fmt.Errorf("fixture %q publishes to the queue but expected_status=%d is not 2xx", fixture.ID, fixture.ExpectedStatus))
		}
	}

	missingBaselines := make([]string, 0)
	for family := range families {
		if baselinesByFamily[family] == 0 {
			missingBaselines = append(missingBaselines, family)
		}
	}
	sort.Strings(missingBaselines)
	for _, family := range missingBaselines {
		parts := strings.SplitN(family, "|", 2)
		issues = append(issues, fmt.Errorf("provider/event family %q/%q must define at least one baseline fixture", parts[0], parts[1]))
	}

	return issues
}
