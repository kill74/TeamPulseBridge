package queue

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestScrubEmails(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			"simple email",
			`{"email": "test@example.com"}`,
			`{"email": "[REDACTED_EMAIL]"}`,
		},
		{
			"multiple emails",
			`{"from": "a@b.com", "to": "c@d.com"}`,
			`{"from": "[REDACTED_EMAIL]", "to": "[REDACTED_EMAIL]"}`,
		},
		{
			"no emails",
			`{"status": "ok"}`,
			`{"status": "ok"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ScrubEmails([]byte(tt.input))
			assert.Equal(t, tt.expected, string(result))
		})
	}
}

func TestScrubTokens(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			"bearer token",
			`{"auth": "Bearer abcd-1234"}`,
			`{"auth": "Bearer [REDACTED_SECRET]"}`,
		},
		{
			"secret key",
			`{"secret": "my-secret-key"}`,
			`{"secret": "[REDACTED_SECRET]"}`,
		},
		{
			"password in query style",
			`password=123456`,
			`password=[REDACTED_SECRET]`,
		},
		{
			"mixed content",
			`{"token":"xyz", "user":"john"}`,
			`{"token":"[REDACTED_SECRET]", "user":"john"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ScrubTokens([]byte(tt.input))
			assert.Equal(t, tt.expected, string(result))
		})
	}
}
