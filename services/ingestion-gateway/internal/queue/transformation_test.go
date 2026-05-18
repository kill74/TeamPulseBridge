package queue

import (
	"context"
	"testing"
)

func TestScrubEmails(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple email",
			input:    "hello user@example.com world",
			expected: "hello [REDACTED_EMAIL] world",
		},
		{
			name:     "multiple emails",
			input:    "test@test.org and admin@domain.co.uk",
			expected: "[REDACTED_EMAIL] and [REDACTED_EMAIL]",
		},
		{
			name:     "no emails",
			input:    "no emails here",
			expected: "no emails here",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "complex email with plus addressing",
			input:    "user.name+tag@sub.domain.com",
			expected: "[REDACTED_EMAIL]",
		},
		{
			name:     "email in JSON",
			input:    `{"email": "user@example.com", "name": "John"}`,
			expected: `{"email": "[REDACTED_EMAIL]", "name": "John"}`,
		},
		{
			name:     "multiple emails in JSON",
			input:    `{"from": "a@b.com", "to": "c@d.com"}`,
			expected: `{"from": "[REDACTED_EMAIL]", "to": "[REDACTED_EMAIL]"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := string(ScrubEmails([]byte(tt.input)))
			if result != tt.expected {
				t.Errorf("ScrubEmails() = %q, want %q", result, tt.expected)
			}
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
			name:     "token in JSON",
			input:    `{"token": "abc123"}`,
			expected: `{"token": "[REDACTED_SECRET]"}`,
		},
		{
			name:     "secret in JSON",
			input:    `{"secret": "my-secret"}`,
			expected: `{"secret": "[REDACTED_SECRET]"}`,
		},
		{
			name:     "password in JSON",
			input:    `{"password": "pass"}`,
			expected: `{"password": "[REDACTED_SECRET]"}`,
		},
		{
			name:     "key in JSON",
			input:    `{"key": "api-key"}`,
			expected: `{"key": "[REDACTED_SECRET]"}`,
		},
		{
			name:     "authorization header",
			input:    `{"authorization": "Bearer xyz"}`,
			expected: `{"authorization": "Bearer [REDACTED_SECRET]"}`,
		},
		{
			name:     "no tokens",
			input:    `no tokens here`,
			expected: `no tokens here`,
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "multiple secrets",
			input:    `{"token":"abc","secret":"def"}`,
			expected: `{"token":"[REDACTED_SECRET]","secret":"[REDACTED_SECRET]"}`,
		},
		{
			name:     "token with equals sign",
			input:    `{"token"="secret123"}`,
			expected: `{"token"="[REDACTED_SECRET]"}`,
		},
		{
			name:     "bearer token format",
			input:    `Authorization: Bearer eyJhbGciOiJIUzI1NiJ9.secret`,
			expected: `Authorization: Bearer [REDACTED_SECRET]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := string(ScrubTokens([]byte(tt.input)))
			if result != tt.expected {
				t.Errorf("ScrubTokens() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestTransformingPublisher(t *testing.T) {
	mock := &mockPublisher{}
	pub := NewTransformingPublisher(mock, ScrubEmails, ScrubTokens)

	body := []byte(`{"email": "user@example.com", "token": "secret123"}`)
	err := pub.Publish(context.Background(), "test", body, nil)
	if err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	expected := `{"email": "[REDACTED_EMAIL]", "token": "[REDACTED_SECRET]"}`
	if string(mock.lastBody) != expected {
		t.Errorf("TransformingPublisher did not transform body correctly")
		t.Errorf("got:  %q", string(mock.lastBody))
		t.Errorf("want: %q", expected)
	}
}

type mockPublisher struct {
	lastBody []byte
}

func (m *mockPublisher) Publish(_ context.Context, _ string, body []byte, _ map[string]string) error {
	m.lastBody = body
	return nil
}

func (m *mockPublisher) Close() error { return nil }

func (m *mockPublisher) HealthCheck(_ context.Context) error { return nil }
