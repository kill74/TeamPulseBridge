package main

import (
	"fmt"
	"os"
	"sort"

	"teampulsebridge/services/ingestion-gateway/internal/testhelpers/fixturecatalog"
)

func main() {
	catalog, err := fixturecatalog.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "fixture lint failed: %v\n", err)
		os.Exit(1)
	}

	issues := catalog.Lint()
	if len(issues) > 0 {
		fmt.Fprintln(os.Stderr, "fixture lint failed:")
		for _, issue := range issues {
			fmt.Fprintf(os.Stderr, " - %v\n", issue)
		}
		os.Exit(1)
	}

	summary := catalog.CoverageSummary()
	fmt.Printf("fixture lint passed: providers=%d families=%d fixtures=%d publishable=%d\n",
		summary.Providers,
		summary.Families,
		summary.TotalFixtures,
		summary.Publishable,
	)
	providers := make([]string, 0, len(summary.ByProvider))
	for provider := range summary.ByProvider {
		providers = append(providers, provider)
	}
	sort.Strings(providers)
	for _, provider := range providers {
		coverage := summary.ByProvider[provider]
		fmt.Printf("  - %s: fixtures=%d families=%d publishable=%d\n",
			provider,
			coverage.Fixtures,
			coverage.Families,
			coverage.Publishable,
		)
	}
}
