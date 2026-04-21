package fixturecatalog

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

const catalogFileName = "catalog-v1.json"

type Catalog struct {
	CatalogVersion int               `json:"catalog_version"`
	EnvelopeSchema EnvelopeSchemaRef `json:"envelope_schema"`
	Fixtures       []Fixture         `json:"fixtures"`
}

type EnvelopeSchemaRef struct {
	Name    string `json:"name"`
	Version int    `json:"version"`
	File    string `json:"file"`
}

type Fixture struct {
	ID             string `json:"id"`
	Version        int    `json:"version"`
	Provider       string `json:"provider"`
	EventFamily    string `json:"event_family"`
	Variant        string `json:"variant"`
	Path           string `json:"path"`
	Negative       bool   `json:"negative"`
	ExpectedStatus int    `json:"expected_status"`
	Publish        bool   `json:"publish"`
	Notes          string `json:"notes"`
}

func Load() (Catalog, error) {
	var catalog Catalog
	raw, err := os.ReadFile(filepath.Join(contractsDir(), catalogFileName))
	if err != nil {
		return catalog, fmt.Errorf("read fixture catalog: %w", err)
	}
	if err := json.Unmarshal(raw, &catalog); err != nil {
		return catalog, fmt.Errorf("unmarshal fixture catalog: %w", err)
	}
	if err := catalog.Validate(); err != nil {
		return catalog, err
	}
	return catalog, nil
}

func (c Catalog) Validate() error {
	if c.CatalogVersion < 1 {
		return errors.New("catalog_version must be >= 1")
	}
	if strings.TrimSpace(c.EnvelopeSchema.Name) == "" {
		return errors.New("envelope_schema.name must not be empty")
	}
	if c.EnvelopeSchema.Version < 1 {
		return errors.New("envelope_schema.version must be >= 1")
	}
	if strings.TrimSpace(c.EnvelopeSchema.File) == "" {
		return errors.New("envelope_schema.file must not be empty")
	}
	if _, err := os.Stat(c.EnvelopeSchemaPath()); err != nil {
		return fmt.Errorf("envelope schema file missing: %w", err)
	}
	if len(c.Fixtures) == 0 {
		return errors.New("fixture catalog must contain at least one fixture")
	}

	seenIDs := make(map[string]struct{}, len(c.Fixtures))
	seenPaths := make(map[string]struct{}, len(c.Fixtures))
	for _, fixture := range c.Fixtures {
		if strings.TrimSpace(fixture.ID) == "" {
			return errors.New("fixture id must not be empty")
		}
		if _, exists := seenIDs[fixture.ID]; exists {
			return fmt.Errorf("duplicate fixture id %q", fixture.ID)
		}
		seenIDs[fixture.ID] = struct{}{}

		if fixture.Version < 1 {
			return fmt.Errorf("fixture %q must declare version >= 1", fixture.ID)
		}
		if strings.TrimSpace(fixture.Provider) == "" {
			return fmt.Errorf("fixture %q provider must not be empty", fixture.ID)
		}
		if strings.TrimSpace(fixture.EventFamily) == "" {
			return fmt.Errorf("fixture %q event_family must not be empty", fixture.ID)
		}
		if strings.TrimSpace(fixture.Variant) == "" {
			return fmt.Errorf("fixture %q variant must not be empty", fixture.ID)
		}
		if strings.TrimSpace(fixture.Path) == "" {
			return fmt.Errorf("fixture %q path must not be empty", fixture.ID)
		}
		if _, exists := seenPaths[fixture.Path]; exists {
			return fmt.Errorf("duplicate fixture path %q", fixture.Path)
		}
		seenPaths[fixture.Path] = struct{}{}
		if fixture.ExpectedStatus < 100 || fixture.ExpectedStatus > 599 {
			return fmt.Errorf("fixture %q expected_status must be between 100 and 599", fixture.ID)
		}

		body, err := ReadFixture(fixture)
		if err != nil {
			return fmt.Errorf("fixture %q: %w", fixture.ID, err)
		}
		if !json.Valid(body) {
			return fmt.Errorf("fixture %q must contain valid JSON", fixture.ID)
		}
	}

	return nil
}

func (c Catalog) EnvelopeSchemaPath() string {
	return filepath.Join(serviceRoot(), filepath.FromSlash(c.EnvelopeSchema.File))
}

func (c Catalog) PublishedFixtures() []Fixture {
	out := make([]Fixture, 0, len(c.Fixtures))
	for _, fixture := range c.Fixtures {
		if fixture.Publish {
			out = append(out, fixture)
		}
	}
	return out
}

func (c Catalog) ProviderFamilies() []string {
	seen := make(map[string]struct{}, len(c.Fixtures))
	out := make([]string, 0, len(c.Fixtures))
	for _, fixture := range c.Fixtures {
		key := strings.ToLower(strings.TrimSpace(fixture.Provider)) + "|" + strings.ToLower(strings.TrimSpace(fixture.EventFamily))
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func ReadFixture(f Fixture) ([]byte, error) {
	path := filepath.Join(contractsDir(), filepath.FromSlash(f.Path))
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read fixture file %q: %w", f.Path, err)
	}
	return body, nil
}

func ListFixtureFiles() ([]string, error) {
	entries, err := os.ReadDir(contractsDir())
	if err != nil {
		return nil, fmt.Errorf("read contracts dir: %w", err)
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if filepath.Ext(name) != ".json" || name == catalogFileName {
			continue
		}
		files = append(files, name)
	}
	sort.Strings(files)
	return files, nil
}

func CompatibilityMatrixPath() string {
	return filepath.Join(serviceRoot(), "docs", "WEBHOOK_COMPATIBILITY_MATRIX.md")
}

func contractsDir() string {
	return filepath.Join(serviceRoot(), "internal", "handlers", "testdata", "contracts")
}

func serviceRoot() string {
	_, currentFile, _, _ := runtime.Caller(0)
	return filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "..", ".."))
}
