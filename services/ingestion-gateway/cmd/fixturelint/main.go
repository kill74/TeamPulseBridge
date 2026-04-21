package main

import (
	"fmt"
	"os"

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

	fmt.Println("fixture lint passed")
}
