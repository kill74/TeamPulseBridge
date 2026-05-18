package queue

import "testing"

func FuzzScrubEmails(f *testing.F) {
	testCases := []string{
		"hello user@example.com world",
		"no emails here",
		"test@test.org and admin@domain.co.uk",
		"",
		"a@b.c",
		"user.name+tag@sub.domain.com",
	}
	for _, tc := range testCases {
		f.Add(tc)
	}
	f.Fuzz(func(t *testing.T, body string) {
		if len(body) > 10000 {
			t.Skip("skipping very long inputs")
		}
		result := ScrubEmails([]byte(body))
		_ = result
	})
}

func FuzzScrubTokens(f *testing.F) {
	testCases := []string{
		`{"token": "abc123"}`,
		`{"secret": "my-secret"}`,
		`{"password": "pass"}`,
		`{"key": "api-key"}`,
		`{"authorization": "Bearer xyz"}`,
		`no tokens here`,
		"",
		`{"token":"abc","secret":"def"}`,
	}
	for _, tc := range testCases {
		f.Add(tc)
	}
	f.Fuzz(func(t *testing.T, body string) {
		if len(body) > 10000 {
			t.Skip("skipping very long inputs")
		}
		result := ScrubTokens([]byte(body))
		_ = result
	})
}
