package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"html/template"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"
)

const uiAssetVersion = "20260324.2"

const (
	uiSmokeMaxBodyBytes         = 256 * 1024
	uiSmokeResponseBodyMaxBytes = 16 * 1024
	uiSmokeMaxRequestsPerMinute = 20
)

var uiSmokeRateLimiter = struct {
	mu      sync.Mutex
	entries map[string]*uiRateEntry
}{
	entries: map[string]*uiRateEntry{},
}

type uiRateEntry struct {
	windowStart time.Time
	count       int
}

type uiSmokeRequest struct {
	Endpoint string            `json:"endpoint"`
	Payload  json.RawMessage   `json:"payload"`
	Headers  map[string]string `json:"headers"`
}

type uiSmokeResponse struct {
	Endpoint string            `json:"endpoint"`
	Status   int               `json:"status"`
	Headers  map[string]string `json:"headers"`
	Body     string            `json:"body"`
}

var productUITemplate = template.Must(template.New("ui").Parse(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>TeamPulse Bridge Console</title>
  <link rel="preconnect" href="https://fonts.googleapis.com">
  <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
  <link href="https://fonts.googleapis.com/css2?family=Space+Grotesk:wght@400;500;600;700&family=IBM+Plex+Mono:wght@400;500&display=swap" rel="stylesheet">
  <link rel="stylesheet" href="/assets/ui.css?v={{.Version}}" />
</head>
<body class="grain">
  <main class="wrap">
    <section class="hero">
      <div class="chip">TeamPulse Bridge</div>
      <h1>Event Ingestion Console for Product and Ops Teams</h1>
      <p class="lede">A live operational view for webhook intake, reliability, and endpoint behavior. Use this console to validate service status, inspect available interfaces, and execute quick smoke checks.</p>
    </section>

    <section class="grid">
      <article class="card span-4 delay-1">
        <h2>Health</h2>
        <div class="kpi">
          <span id="healthDot" class="dot"></span>
          <div>
            <div class="label">GET /healthz</div>
            <div id="healthValue" class="value">Checking...</div>
          </div>
        </div>
        <div class="statline">
          <span id="healthCode" class="pill">status: --</span>
          <span id="healthLatency" class="pill">latency: --</span>
        </div>
      </article>

      <article class="card span-4 delay-2">
        <h2>Readiness</h2>
        <div class="kpi">
          <span id="readyDot" class="dot"></span>
          <div>
            <div class="label">GET /readyz</div>
            <div id="readyValue" class="value">Checking...</div>
          </div>
        </div>
        <div class="statline">
          <span id="readyCode" class="pill">status: --</span>
          <span id="readyLatency" class="pill">latency: --</span>
        </div>
      </article>

      <article class="card span-4 delay-3">
        <h2>Admin Configuration</h2>
        <p class="small">This panel calls <strong>/admin/configz</strong>. If admin JWT is enabled, unauthorized requests are expected from browsers without a token.</p>
        <div class="statline">
          <span id="adminCode" class="pill">status: --</span>
          <span id="adminState" class="pill">state: checking</span>
        </div>
        <div class="field">
          <label class="label" for="token">Operator JWT (optional)</label>
          <input id="token" class="input" type="password" placeholder="Paste Bearer token for admin endpoint access" />
        </div>
        <div class="row row-top-gap">
          <button id="saveToken" class="ghost compact">Save Session Token</button>
          <button id="clearToken" class="ghost compact">Clear Token</button>
        </div>
      </article>

      <article class="card span-8 delay-4">
        <h2>Webhook Smoke Tester</h2>
        <p class="small">Send a test payload to webhook endpoints. Most providers require valid signatures/tokens, so 401/403 responses are expected unless headers are configured correctly.</p>
        <div class="row row-top-gap">
          <input id="allowSend" class="switch" type="checkbox" aria-label="Enable webhook sends" />
          <span class="small">Enable webhook test sends (safety guard)</span>
        </div>

        <div class="actions">
          <button data-endpoint="/webhooks/github">Send GitHub Sample</button>
          <button data-endpoint="/webhooks/gitlab">Send GitLab Sample</button>
          <button data-endpoint="/webhooks/slack">Send Slack Sample</button>
          <button data-endpoint="/webhooks/teams">Send Teams Sample</button>
        </div>

        <div class="field">
          <label class="label" for="extraHeaders">Extra Headers (JSON object, optional)</label>
          <textarea id="extraHeaders" class="input" rows="4">{
  "X-Demo-Source": "ui-console"
}</textarea>
        </div>

        <div class="field">
          <label class="label" for="payload">Payload</label>
          <textarea id="payload" class="input" rows="7">{
  "type": "demo_event",
  "source": "ui-console",
  "timestamp": "2026-03-24T00:00:00Z"
}</textarea>
        </div>

        <div class="field">
          <label class="label" for="result">Response</label>
          <pre id="result" class="code">Ready.</pre>
        </div>
      </article>

      <article class="card span-4 delay-5">
        <h2>Operator Controls</h2>
        <div class="row row-bottom-gap">
          <input id="autoRefresh" class="switch" type="checkbox" checked aria-label="Toggle auto refresh" />
          <span class="small">Auto refresh every 15s</span>
        </div>
        <div class="actions actions-single no-top-gap">
          <button id="manualRefresh">Refresh Now</button>
        </div>

        <h2 class="stack-heading">Quick Access</h2>
        <p class="small">Open core operational endpoints in a new tab.</p>
        <div class="actions actions-single">
          <a class="ghost" href="/metrics" target="_blank" rel="noreferrer">Open Metrics</a>
          <a class="ghost" href="/healthz" target="_blank" rel="noreferrer">Open Health</a>
          <a class="ghost" href="/readyz" target="_blank" rel="noreferrer">Open Readiness</a>
          <a class="ghost" href="/admin/configz" target="_blank" rel="noreferrer">Open Admin Config</a>
        </div>
      </article>
    </section>

    <p class="footer">This UI is rendered by the Go service and secured with strict browser headers for production environments.</p>
  </main>

  <script defer src="/assets/ui.js?v={{.Version}}"></script>
</body>
</html>`))

const productUICSS = `:root {
  --bg: #0d1321;
  --panel: #17233b;
  --ink: #e9eef8;
  --muted: #9fb0cd;
  --line: #2f4163;
  --ok: #36d399;
  --warn: #ffbd4a;
  --err: #ff6b6b;
  --accent: #2dd4bf;
  --shadow: 0 16px 48px rgba(0, 0, 0, 0.35);
  --radius: 16px;
}

* { box-sizing: border-box; }

html, body {
  margin: 0;
  min-height: 100%;
  background:
    radial-gradient(1200px 700px at 90% -5%, rgba(45, 212, 191, 0.22), transparent 60%),
    radial-gradient(900px 600px at -10% 10%, rgba(34, 197, 94, 0.2), transparent 60%),
    linear-gradient(165deg, #0a1020 0%, #111b2f 55%, #0e1728 100%);
  color: var(--ink);
  font-family: "Space Grotesk", "Segoe UI", sans-serif;
}

.grain::before {
  content: "";
  position: fixed;
  inset: 0;
  pointer-events: none;
  opacity: 0.08;
  background-image: radial-gradient(#ffffff 0.65px, transparent 0.65px);
  background-size: 3px 3px;
  z-index: 0;
}

.wrap {
  position: relative;
  z-index: 1;
  width: min(1180px, 92vw);
  margin: 0 auto;
  padding: 34px 0 56px;
}

.hero {
  display: grid;
  gap: 18px;
  margin-bottom: 24px;
  animation: rise 540ms cubic-bezier(.2,.8,.2,1) both;
}

.chip {
  width: fit-content;
  border: 1px solid rgba(157, 181, 219, 0.35);
  background: rgba(23, 35, 59, 0.62);
  backdrop-filter: blur(3px);
  color: #d6e3fb;
  border-radius: 999px;
  padding: 7px 12px;
  font: 500 12px/1 "IBM Plex Mono", monospace;
  letter-spacing: 0.06em;
  text-transform: uppercase;
}

h1 {
  margin: 0;
  font-size: clamp(1.8rem, 3.5vw, 3.25rem);
  line-height: 1.06;
  max-width: 16ch;
  text-wrap: balance;
}

h2 {
  margin: 0 0 10px;
  font-size: 1rem;
  letter-spacing: 0.02em;
}

.lede {
  margin: 0;
  max-width: 70ch;
  color: var(--muted);
  font-size: clamp(0.98rem, 1.2vw, 1.1rem);
}

.grid {
  display: grid;
  grid-template-columns: repeat(12, 1fr);
  gap: 14px;
}

.card {
  background: linear-gradient(180deg, rgba(30, 44, 72, 0.92), rgba(20, 31, 53, 0.95));
  border: 1px solid var(--line);
  border-radius: var(--radius);
  padding: 16px;
  box-shadow: var(--shadow);
  animation: rise 560ms cubic-bezier(.2,.8,.2,1) both;
}

.delay-1 { animation-delay: 80ms; }
.delay-2 { animation-delay: 150ms; }
.delay-3 { animation-delay: 220ms; }
.delay-4 { animation-delay: 260ms; }
.delay-5 { animation-delay: 320ms; }

.small {
  color: var(--muted);
  font-size: 0.92rem;
  line-height: 1.45;
}

.span-4 { grid-column: span 4; }
.span-8 { grid-column: span 8; }

.kpi {
  display: grid;
  grid-template-columns: auto 1fr;
  gap: 10px;
  align-items: center;
}

.dot {
  width: 12px;
  height: 12px;
  border-radius: 50%;
  background: var(--warn);
  box-shadow: 0 0 0 0 rgba(255, 189, 74, 0.45);
  animation: pulse 1800ms ease-out infinite;
}

.dot.ok { background: var(--ok); box-shadow: 0 0 0 0 rgba(54, 211, 153, 0.45); }
.dot.err { background: var(--err); box-shadow: 0 0 0 0 rgba(255, 107, 107, 0.5); }

.label {
  color: var(--muted);
  font-size: 0.82rem;
  text-transform: uppercase;
  letter-spacing: 0.08em;
  margin-bottom: 3px;
  font-family: "IBM Plex Mono", monospace;
}

.value {
  font-size: 1.15rem;
  font-weight: 700;
}

.statline {
  margin-top: 12px;
  display: flex;
  gap: 9px;
  flex-wrap: wrap;
}

.pill {
  border: 1px solid var(--line);
  border-radius: 999px;
  padding: 6px 9px;
  color: var(--muted);
  font: 500 12px/1 "IBM Plex Mono", monospace;
  background: rgba(13, 19, 33, 0.55);
}

.actions {
  margin-top: 10px;
  display: grid;
  gap: 8px;
  grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
}

.actions-single { grid-template-columns: 1fr; }

button, .ghost {
  cursor: pointer;
  border: 1px solid transparent;
  border-radius: 11px;
  padding: 10px 12px;
  color: #06131f;
  font-family: "Space Grotesk", sans-serif;
  font-weight: 700;
  background: linear-gradient(130deg, var(--accent), #7deac0);
  transition: transform .16s ease, filter .16s ease;
  text-align: center;
  text-decoration: none;
  display: inline-block;
}

.ghost {
  background: transparent;
  color: var(--ink);
  border-color: var(--line);
  font-weight: 600;
}

.compact { padding: 8px 10px; }

button:hover, .ghost:hover { transform: translateY(-1px); filter: brightness(1.03); }

.code {
  margin: 0;
  white-space: pre-wrap;
  font: 500 12px/1.45 "IBM Plex Mono", monospace;
  background: rgba(11, 17, 29, 0.8);
  border: 1px solid var(--line);
  color: #d8e6ff;
  border-radius: 12px;
  padding: 11px;
  min-height: 146px;
}

.input {
  width: 100%;
  border-radius: 11px;
  border: 1px solid var(--line);
  background: #0f1a2e;
  color: var(--ink);
  padding: 10px 11px;
  font: 500 14px/1.45 "IBM Plex Mono", monospace;
  outline: none;
}

.input:focus {
  border-color: #4fe7d2;
  box-shadow: 0 0 0 3px rgba(79, 231, 210, 0.18);
}

.footer {
  margin-top: 18px;
  color: var(--muted);
  font-size: 0.9rem;
}

.field {
  display: grid;
  gap: 6px;
  margin-top: 10px;
}

.row {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
  align-items: center;
}

.row-top-gap { margin-top: 8px; }
.row-bottom-gap { margin-bottom: 8px; }
.no-top-gap { margin-top: 0; }
.stack-heading { margin-top: 14px; }

.switch {
  appearance: none;
  width: 42px;
  height: 24px;
  border-radius: 999px;
  border: 1px solid var(--line);
  background: #122038;
  position: relative;
  transition: background .2s ease;
  cursor: pointer;
}

.switch::before {
  content: "";
  position: absolute;
  top: 3px;
  left: 3px;
  width: 16px;
  height: 16px;
  border-radius: 50%;
  background: #dbe9ff;
  transition: transform .2s ease;
}

.switch:checked {
  background: #1e7f68;
  border-color: #32bea0;
}

.switch:checked::before {
  transform: translateX(18px);
}

@keyframes rise {
  from { opacity: 0; transform: translateY(10px) scale(0.995); }
  to { opacity: 1; transform: translateY(0) scale(1); }
}

@keyframes pulse {
  0% { box-shadow: 0 0 0 0 currentColor; }
  70% { box-shadow: 0 0 0 10px rgba(0,0,0,0); }
  100% { box-shadow: 0 0 0 0 rgba(0,0,0,0); }
}

@media (max-width: 960px) {
  .span-4, .span-8 { grid-column: span 12; }
  .wrap { width: min(1180px, 94vw); }
}
`

const productUIJS = `
var refreshTimer = null;

function authHeader() {
  var token = document.getElementById('token').value.trim();
  if (!token) {
    return {};
  }
  return { 'Authorization': token.startsWith('Bearer ') ? token : ('Bearer ' + token) };
}

function setStatus(prefix, data) {
  var dot = document.getElementById(prefix + 'Dot');
  var value = document.getElementById(prefix + 'Value');
  var code = document.getElementById(prefix + 'Code');
  var latency = document.getElementById(prefix + 'Latency');

  code.textContent = 'status: ' + data.status;
  latency.textContent = 'latency: ' + data.latency + 'ms';

  if (data.ok) {
    dot.className = 'dot ok';
    value.textContent = 'Operational';
  } else {
    dot.className = 'dot err';
    value.textContent = 'Issue Detected';
  }
}

async function check(path, timeoutMs, withAuth) {
  var started = performance.now();
  var controller = new AbortController();
  var timer = setTimeout(function() { controller.abort(); }, timeoutMs);
  try {
    var headers = withAuth ? authHeader() : {};
    var res = await fetch(path, { signal: controller.signal, headers: headers });
    return {
      status: res.status,
      ok: res.ok,
      latency: Math.round(performance.now() - started)
    };
  } catch (_) {
    return {
      status: 'ERR',
      ok: false,
      latency: Math.round(performance.now() - started)
    };
  } finally {
    clearTimeout(timer);
  }
}

async function refreshStatus() {
  var health = await check('/healthz', 2200, false);
  var ready = await check('/readyz', 2200, false);

  setStatus('health', health);
  setStatus('ready', ready);

  var admin = await check('/admin/configz', 2600, true);
  var adminCode = document.getElementById('adminCode');
  var adminState = document.getElementById('adminState');
  adminCode.textContent = 'status: ' + admin.status;
  if (admin.status === 200) {
    adminState.textContent = 'state: visible';
  } else if (admin.status === 401 || admin.status === 403) {
    adminState.textContent = 'state: locked (auth required)';
  } else {
    adminState.textContent = 'state: unavailable';
  }
}

function parseExtraHeaders() {
  var raw = document.getElementById('extraHeaders').value.trim();
  if (!raw) {
    return {};
  }
  var parsed = JSON.parse(raw);
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
    throw new Error('Extra headers must be a JSON object');
  }
  var out = {};
  Object.keys(parsed).forEach(function(key) {
    out[String(key)] = String(parsed[key]);
  });
  return out;
}

async function sendSample(endpoint) {
  var payload = document.getElementById('payload').value;
  var result = document.getElementById('result');

  if (!document.getElementById('allowSend').checked) {
    result.textContent = 'Send blocked by safety guard. Enable "webhook test sends" first.';
    return;
  }

  result.textContent = 'Sending to ' + endpoint + '...';
  try {
    var headers = Object.assign({ 'Content-Type': 'application/json' }, authHeader(), parseExtraHeaders());
    var res = await fetch('/ui/smoke-test', {
      method: 'POST',
      headers: authHeader(),
      body: JSON.stringify({
        endpoint: endpoint,
        payload: payload,
        headers: headers
      })
    });
    var body = await res.text();
    var parsed;
    try {
      parsed = JSON.parse(body);
    } catch (_) {
      parsed = { status: res.status, body: body };
    }
    result.textContent = [
      'Endpoint: ' + endpoint,
      'Status: ' + (parsed.status || res.status),
      '',
      (parsed.body || body || '(no response body)'),
      '',
      'Note: webhook providers usually require valid signatures/tokens.'
    ].join('\n');
  } catch (err) {
    result.textContent = 'Request failed: ' + (err && err.message ? err.message : String(err));
  }
}

function loadToken() {
  var saved = sessionStorage.getItem('tpb.ui.jwt') || '';
  document.getElementById('token').value = saved;
}

function persistToken() {
  var token = document.getElementById('token').value.trim();
  if (!token) {
    sessionStorage.removeItem('tpb.ui.jwt');
    return;
  }
  sessionStorage.setItem('tpb.ui.jwt', token);
}

function setupAutoRefresh() {
  if (refreshTimer) {
    clearInterval(refreshTimer);
  }
  if (document.getElementById('autoRefresh').checked) {
    refreshTimer = setInterval(refreshStatus, 15000);
  }
}

document.querySelectorAll('button[data-endpoint]').forEach(function(btn) {
  btn.addEventListener('click', function() {
    sendSample(btn.getAttribute('data-endpoint'));
  });
});

document.getElementById('manualRefresh').addEventListener('click', refreshStatus);
document.getElementById('autoRefresh').addEventListener('change', setupAutoRefresh);
document.getElementById('saveToken').addEventListener('click', function() {
  persistToken();
  refreshStatus();
});
document.getElementById('clearToken').addEventListener('click', function() {
  document.getElementById('token').value = '';
  sessionStorage.removeItem('tpb.ui.jwt');
  refreshStatus();
});

loadToken();
refreshStatus();
setupAutoRefresh();
`

func setUISecurityHeaders(w http.ResponseWriter) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
	w.Header().Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
	w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
	w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
	w.Header().Set("Origin-Agent-Cluster", "?1")
}

func setUICSPHeader(w http.ResponseWriter) {
	csp := "default-src 'self'; " +
		"script-src 'self'; " +
		"style-src 'self' https://fonts.googleapis.com; " +
		"font-src 'self' https://fonts.gstatic.com; " +
		"img-src 'self' data:; " +
		"connect-src 'self'; " +
		"base-uri 'none'; " +
		"frame-ancestors 'none'; " +
		"form-action 'self'"
	w.Header().Set("Content-Security-Policy", csp)
}

func setAssetCachingHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "public, max-age=86400, immutable")
	w.Header().Set("ETag", `"ui-`+uiAssetVersion+`"`)
}

func isNotModified(r *http.Request, w http.ResponseWriter) bool {
	if match := r.Header.Get("If-None-Match"); match == `"ui-`+uiAssetVersion+`"` {
		w.WriteHeader(http.StatusNotModified)
		return true
	}
	return false
}

// ProductUI serves the operator console page.
func ProductUI(w http.ResponseWriter, r *http.Request) {
	setUISecurityHeaders(w)
	setUICSPHeader(w)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")

	_ = productUITemplate.Execute(w, struct{ Version string }{Version: uiAssetVersion})
	_ = r
}

// ProductUIStyles serves versioned CSS for the product UI.
func ProductUIStyles(w http.ResponseWriter, r *http.Request) {
	setUISecurityHeaders(w)
	setAssetCachingHeaders(w)
	if isNotModified(r, w) {
		return
	}
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	_, _ = w.Write([]byte(productUICSS))
}

// ProductUIScript serves versioned JS for the product UI.
func ProductUIScript(w http.ResponseWriter, r *http.Request) {
	setUISecurityHeaders(w)
	setAssetCachingHeaders(w)
	if isNotModified(r, w) {
		return
	}
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	_, _ = w.Write([]byte(productUIJS))
}

func clientIP(remoteAddr string) string {
	ip := strings.TrimSpace(remoteAddr)
	if host, _, err := net.SplitHostPort(ip); err == nil && host != "" {
		return strings.Trim(host, "[]")
	}
	if strings.HasPrefix(ip, "[") && strings.HasSuffix(ip, "]") {
		return strings.TrimSuffix(strings.TrimPrefix(ip, "["), "]")
	}
	return ip
}

func allowUISmokeRequest(ip string, now time.Time) bool {
	uiSmokeRateLimiter.mu.Lock()
	defer uiSmokeRateLimiter.mu.Unlock()

	if ip == "" {
		ip = "unknown"
	}

	entry := uiSmokeRateLimiter.entries[ip]
	if entry == nil {
		uiSmokeRateLimiter.entries[ip] = &uiRateEntry{windowStart: now, count: 1}
		return true
	}

	if now.Sub(entry.windowStart) >= time.Minute {
		entry.windowStart = now
		entry.count = 1
		return true
	}

	if entry.count >= uiSmokeMaxRequestsPerMinute {
		return false
	}

	entry.count++
	return true
}

func sanitizeUISmokeHeaders(headers map[string]string) map[string]string {
	clean := map[string]string{}
	for k, v := range headers {
		key := strings.TrimSpace(k)
		value := strings.TrimSpace(v)
		if key == "" || value == "" {
			continue
		}
		// Allow content type and provider signature/token headers for smoke tests.
		lower := strings.ToLower(key)
		if lower == "content-type" || strings.HasPrefix(lower, "x-") || lower == "authorization" {
			clean[key] = value
		}
	}
	if _, ok := clean["Content-Type"]; !ok {
		clean["Content-Type"] = "application/json"
	}
	return clean
}

func isAllowedSmokeEndpoint(endpoint string) bool {
	switch endpoint {
	case "/webhooks/slack", "/webhooks/teams", "/webhooks/github", "/webhooks/gitlab":
		return true
	default:
		return false
	}
}

// NewUISmokeTestProxy returns an internal proxy for controlled webhook smoke testing.
func NewUISmokeTestProxy(target http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		setUISecurityHeaders(w)
		w.Header().Set("Cache-Control", "no-store")

		if !allowUISmokeRequest(clientIP(r.RemoteAddr), time.Now().UTC()) {
			writeJSON(w, http.StatusTooManyRequests, map[string]string{
				"error": "rate limit exceeded for UI smoke tests",
			})
			return
		}

		bodyReader := io.LimitReader(r.Body, uiSmokeMaxBodyBytes)
		defer r.Body.Close()

		var req uiSmokeRequest
		if err := json.NewDecoder(bodyReader).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON payload"})
			return
		}

		req.Endpoint = strings.TrimSpace(req.Endpoint)
		if !isAllowedSmokeEndpoint(req.Endpoint) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "endpoint not allowed"})
			return
		}
		if len(req.Payload) == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "payload is required"})
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		defer cancel()

		internalReq, err := http.NewRequestWithContext(ctx, http.MethodPost, req.Endpoint, bytes.NewReader(req.Payload))
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to build internal request"})
			return
		}
		internalReq.RemoteAddr = r.RemoteAddr
		for k, v := range sanitizeUISmokeHeaders(req.Headers) {
			internalReq.Header.Set(k, v)
		}

		rr := httptest.NewRecorder()
		target.ServeHTTP(rr, internalReq)

		respBody := rr.Body.String()
		if len(respBody) > uiSmokeResponseBodyMaxBytes {
			respBody = respBody[:uiSmokeResponseBodyMaxBytes] + "\n...truncated"
		}

		resp := uiSmokeResponse{
			Endpoint: req.Endpoint,
			Status:   rr.Code,
			Headers: map[string]string{
				"Content-Type": rr.Header().Get("Content-Type"),
				"X-Request-Id": rr.Header().Get("X-Request-Id"),
			},
			Body: respBody,
		}

		writeJSON(w, http.StatusOK, resp)
	}
}
