package signature

import (
	"testing"
	"time"
)

func FuzzValidateSlack(f *testing.F) {
	testCases := []struct {
		secret    string
		timestamp string
		body      string
		signature string
	}{
		{"secret", "1234567890", `{"type":"event"}`, "v0=abc123"},
		{"", "", "", ""},
		{"a", "b", "c", "d"},
	}
	for _, tc := range testCases {
		f.Add(tc.secret, tc.timestamp, tc.body, tc.signature)
	}
	f.Fuzz(func(t *testing.T, secret, timestamp, body, signature string) {
		if len(body) > 10000 {
			t.Skip("skipping very long bodies")
		}
		err := ValidateSlack(secret, timestamp, body, signature, time.Now(), 5*time.Minute)
		_ = err
	})
}

func FuzzValidateGitHub(f *testing.F) {
	testCases := []struct {
		secret    string
		body      string
		signature string
	}{
		{"secret", `{"action":"opened"}`, "sha256=abc123"},
		{"", "", ""},
		{"a", "b", "c"},
	}
	for _, tc := range testCases {
		f.Add(tc.secret, tc.body, tc.signature)
	}
	f.Fuzz(func(t *testing.T, secret, body, signature string) {
		if len(body) > 10000 {
			t.Skip("skipping very long bodies")
		}
		err := ValidateGitHub(secret, []byte(body), signature)
		_ = err
	})
}

func FuzzValidateGitLab(f *testing.F) {
	testCases := []struct {
		token  string
		header string
	}{
		{"secret-token", "secret-token"},
		{"", ""},
		{"a", "b"},
	}
	for _, tc := range testCases {
		f.Add(tc.token, tc.header)
	}
	f.Fuzz(func(t *testing.T, token, header string) {
		err := ValidateGitLab(token, header)
		_ = err
	})
}

func FuzzValidateTeamsClientState(f *testing.F) {
	testCases := []struct {
		expected string
		actual   string
	}{
		{"state-123", "state-123"},
		{"", ""},
		{"a", "b"},
	}
	for _, tc := range testCases {
		f.Add(tc.expected, tc.actual)
	}
	f.Fuzz(func(t *testing.T, expected, actual string) {
		err := ValidateTeamsClientState(expected, actual)
		_ = err
	})
}
