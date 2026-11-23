package httpx

import (
	"context"
	"testing"
	"time"
)

func TestCombinedCheckFailOpenOnNilClient(t *testing.T) {
	l := NewRedisRateLimiterWithTimeout(nil, "rate_limit", time.Minute, 50*time.Millisecond)
	res := l.CombinedCheckWithContext(context.Background(), "general|1.2.3.4", "source|github|1.2.3.4", "webhook_dedup:github:abc", 300, 100, 5*time.Minute)
	if !res.GeneralAllowed || !res.SourceAllowed || res.IsDuplicate {
		t.Fatalf("expected fail-open allow, got %+v", res)
	}
}

func TestCombinedGateSkipsDedupWhenNoEventID(t *testing.T) {
	l := NewRedisRateLimiterWithTimeout(nil, "rate_limit", time.Minute, 50*time.Millisecond)
	res := l.CombinedCheckWithContext(context.Background(), "general|1.2.3.4", "source|github|1.2.3.4", "", 300, 100, 5*time.Minute)
	if res.IsDuplicate {
		t.Fatalf("empty dedup key must never report duplicate, got %+v", res)
	}
}

func TestSourceFromWebhookPath(t *testing.T) {
	cases := map[string]string{
		"/webhooks/github":        "github",
		"/api/v1/webhooks/slack":  "slack",
		"/webhooks/gitlab/extra":  "gitlab",
		"/admin/configz":          "",
		"/healthz":                "",
		"/api/v1/admin/flags":     "",
		"/webhooks/":              "",
		"/api/v1/webhooks/teams/": "teams",
		"/WEBHOOKS/github":        "",
	}
	for path, want := range cases {
		if got := sourceFromWebhookPath(path); got != want {
			t.Fatalf("sourceFromWebhookPath(%q)=%q, want %q", path, got, want)
		}
	}
}
