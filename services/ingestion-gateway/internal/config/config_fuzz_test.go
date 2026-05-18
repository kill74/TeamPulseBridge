package config

import "testing"

func FuzzIsWeakSecret(f *testing.F) {
	testCases := []string{
		"change-me",
		"changeme",
		"secret",
		"password",
		"admin",
		"test",
		"default",
		"",
		"aaaaaaaaaa",
		"1234567890",
		"a-very-strong-secret-key-that-is-not-weak-at-all-12345",
		"x",
		"   ",
	}
	for _, tc := range testCases {
		f.Add(tc)
	}
	f.Fuzz(func(t *testing.T, secret string) {
		result := isWeakSecret(secret)
		if secret == "" {
			if !result {
				t.Errorf("expected empty string to be weak, got false")
			}
		}
		if len(secret) > 1000 {
			t.Skip("skipping very long secrets")
		}
		_ = result
	})
}

func FuzzParseSourceRateLimits(f *testing.F) {
	testCases := []string{
		"github:100,slack:200",
		"github:100",
		"",
		"invalid",
		"github:abc",
		"github:-1",
		"github:0",
		"github:100,slack:200,gitlab:300",
		":100",
		"github:",
	}
	for _, tc := range testCases {
		f.Add(tc)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		result := parseSourceRateLimits(raw)
		if len(raw) > 10000 {
			t.Skip("skipping very long inputs")
		}
		_ = result
	})
}

func FuzzBoolOrDefault(f *testing.F) {
	testCases := []string{
		"true",
		"false",
		"1",
		"0",
		"yes",
		"no",
		"y",
		"n",
		"",
		"TRUE",
		"False",
		"maybe",
		"  true  ",
	}
	for _, tc := range testCases {
		f.Add(tc)
	}
	f.Fuzz(func(t *testing.T, input string) {
		result := boolOrDefault(input, false)
		_ = result
	})
}

func FuzzSplitCSVEnv(f *testing.F) {
	testCases := []string{
		"10.0.0.0/8,172.16.0.0/12",
		"10.0.0.0/8",
		"",
		",,",
		"  10.0.0.0/8  ,  172.16.0.0/12  ",
		"10.0.0.0/8,,172.16.0.0/12",
	}
	for _, tc := range testCases {
		f.Add(tc)
	}
	f.Fuzz(func(t *testing.T, input string) {
		result := splitCSVEnv(input)
		_ = result
	})
}
