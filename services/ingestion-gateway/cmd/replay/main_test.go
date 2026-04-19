package main

import "testing"

func TestKVFlagsSet(t *testing.T) {
	var flags kvFlags
	if err := flags.Set("X-Test=true"); err != nil {
		t.Fatalf("set flag: %v", err)
	}
	if got := flags["X-Test"]; got != "true" {
		t.Fatalf("expected header value true, got %q", got)
	}
}

func TestKVFlagsSetRejectsMissingEquals(t *testing.T) {
	var flags kvFlags
	if err := flags.Set("invalid"); err == nil {
		t.Fatal("expected invalid header format error")
	}
}
