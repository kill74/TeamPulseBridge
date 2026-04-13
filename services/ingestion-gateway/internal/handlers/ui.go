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

const uiAssetVersion = "20260325.4"

const (
	uiSmokeMaxBodyBytes         = 256 * 1024
	uiSmokeResponseBodyMaxBytes = 16 * 1024
	uiSmokeMaxRequestsPerMinute = 20
)

var uiSmokeRateLimiter = struct {
	mu          sync.Mutex
	entries     map[string]*uiRateEntry
	lastCleanup time.Time
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
<body class="grain aurora">
  <main class="wrap">
    <div class="ambient ambient-a"></div>
    <div class="ambient ambient-b"></div>

    <section class="hero">
      <div class="chip">TeamPulse Bridge</div>
      <h1>Event Ingestion Console for Product and Ops Teams</h1>
      <p class="lede">A live operational view for webhook intake, reliability, and endpoint behavior. Use this console to validate service status, inspect available interfaces, and execute quick smoke checks.</p>
      <div class="hero-meta">
        <span id="overallState" class="pill pill-hero">service: checking</span>
        <span id="refreshMode" class="pill pill-hero">auto refresh: on</span>
        <span id="lastRefresh" class="pill pill-hero">last refresh: --</span>
      </div>
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
          <span id="healthSla" class="pill">sla(20): --</span>
        </div>
        <div class="trend">
          <div class="label">Latency Trend (last 20 checks)</div>
          <svg id="healthTrend" class="trend-svg" viewBox="0 0 260 56" preserveAspectRatio="none" role="img" aria-label="Health latency trend">
            <path d="" />
          </svg>
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
          <span id="readySla" class="pill">sla(20): --</span>
        </div>
        <div class="trend">
          <div class="label">Latency Trend (last 20 checks)</div>
          <svg id="readyTrend" class="trend-svg" viewBox="0 0 260 56" preserveAspectRatio="none" role="img" aria-label="Readiness latency trend">
            <path d="" />
          </svg>
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
          <span id="smokeGuardState" class="pill">guard: locked</span>
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
          <pre id="result" class="code" role="status" aria-live="polite">Ready.</pre>
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

        <h2 class="stack-heading">Keyboard Shortcuts</h2>
        <p class="small">R = refresh status, S = save token, C = clear token. Disabled while typing in fields.</p>
      </article>

      <article class="card span-12 delay-5">
        <h2>Incident Timeline</h2>
        <p class="small">Recent operational transitions detected by this browser session.</p>
        <ul id="incidentLog" class="timeline" aria-live="polite">
          <li class="timeline-item muted">Waiting for first status sample...</li>
        </ul>
      </article>
    </section>

    <p class="footer">Rendered by the Go service with strict browser security headers and zero external UI runtime dependencies.</p>
  </main>

  <script defer src="/assets/ui.js?v={{.Version}}"></script>
</body>
</html>`))

const productUICSS = `:root {
  --bg: #f7f8f5;
  --ink: #1f2430;
  --muted: #596277;
  --line: #d4d8e2;
  --panel: rgba(255, 255, 255, 0.82);
  --ok: #169c6a;
  --warn: #dc8c00;
  --err: #d64545;
  --accent: #0f6ad6;
  --accent-2: #f66e3c;
  --shadow: 0 20px 45px rgba(31, 36, 48, 0.12);
  --radius: 18px;
}

* { box-sizing: border-box; }

html, body {
  margin: 0;
  min-height: 100%;
  background:
    radial-gradient(1200px 620px at -8% -12%, rgba(246, 110, 60, 0.22), transparent 62%),
    radial-gradient(940px 580px at 108% -16%, rgba(15, 106, 214, 0.2), transparent 62%),
    linear-gradient(180deg, #f8f8f6 0%, #f2f5fb 55%, #ecf2ff 100%);
  color: var(--ink);
  font-family: "Space Grotesk", "Segoe UI", sans-serif;
}

.grain::before {
  content: "";
  position: fixed;
  inset: 0;
  pointer-events: none;
  opacity: 0.06;
  background-image: radial-gradient(#1f2430 0.55px, transparent 0.55px);
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

.ambient {
  position: absolute;
  border-radius: 999px;
  filter: blur(28px);
  z-index: -1;
  pointer-events: none;
}

.ambient-a {
  width: 300px;
  height: 300px;
  top: 20px;
  right: -70px;
  background: rgba(15, 106, 214, 0.22);
}

.ambient-b {
  width: 260px;
  height: 260px;
  bottom: 40px;
  left: -80px;
  background: rgba(246, 110, 60, 0.2);
}

.hero {
  display: grid;
  gap: 16px;
  margin-bottom: 24px;
  animation: rise 560ms cubic-bezier(.2,.8,.2,1) both;
}

.chip {
  width: fit-content;
  border: 1px solid rgba(89, 98, 119, 0.32);
  background: rgba(255, 255, 255, 0.8);
  backdrop-filter: blur(5px);
  color: #2e3649;
  border-radius: 999px;
  padding: 7px 12px;
  font: 500 12px/1 "IBM Plex Mono", monospace;
  letter-spacing: 0.06em;
  text-transform: uppercase;
}

h1 {
  margin: 0;
  font-size: clamp(1.9rem, 3.6vw, 3.3rem);
  line-height: 1.04;
  max-width: 17ch;
  text-wrap: balance;
}

h2 {
  margin: 0 0 10px;
  font-size: 1rem;
  letter-spacing: 0.02em;
}

.lede {
  margin: 0;
  max-width: 72ch;
  color: var(--muted);
  font-size: clamp(0.98rem, 1.2vw, 1.08rem);
}

.hero-meta {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

.pill-hero {
  background: rgba(255, 255, 255, 0.88);
}

.grid {
  display: grid;
  grid-template-columns: repeat(12, minmax(0, 1fr));
  gap: 14px;
}

.card {
  background: var(--panel);
  border: 1px solid rgba(255, 255, 255, 0.74);
  border-radius: var(--radius);
  padding: 16px;
  box-shadow: var(--shadow);
  backdrop-filter: blur(8px);
  animation: rise 580ms cubic-bezier(.2,.8,.2,1) both;
}

.delay-1 { animation-delay: 90ms; }
.delay-2 { animation-delay: 160ms; }
.delay-3 { animation-delay: 220ms; }
.delay-4 { animation-delay: 280ms; }
.delay-5 { animation-delay: 340ms; }

.small {
  color: var(--muted);
  font-size: 0.92rem;
  line-height: 1.45;
}

.span-4 { grid-column: span 4; }
.span-8 { grid-column: span 8; }
.span-12 { grid-column: span 12; }

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
  box-shadow: 0 0 0 0 rgba(220, 140, 0, 0.4);
  animation: pulse 1800ms ease-out infinite;
}

.dot.ok { background: var(--ok); box-shadow: 0 0 0 0 rgba(22, 156, 106, 0.42); }
.dot.err { background: var(--err); box-shadow: 0 0 0 0 rgba(214, 69, 69, 0.38); }

.label {
  color: var(--muted);
  font-size: 0.8rem;
  text-transform: uppercase;
  letter-spacing: 0.08em;
  margin-bottom: 3px;
  font-family: "IBM Plex Mono", monospace;
}

.value {
  font-size: 1.16rem;
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
  color: #525d75;
  font: 500 12px/1 "IBM Plex Mono", monospace;
  background: #f8fbff;
}

.pill.ok {
  color: #0f7f56;
  background: #eefcf6;
  border-color: #bae8d3;
}

.pill.warn {
  color: #a45d00;
  background: #fff7e8;
  border-color: #f2d7ab;
}

.pill.err {
  color: #a52e2e;
  background: #fff1f1;
  border-color: #efc1c1;
}

.actions {
  margin-top: 10px;
  display: grid;
  gap: 8px;
  grid-template-columns: repeat(auto-fit, minmax(170px, 1fr));
}

.actions-single { grid-template-columns: 1fr; }

button, .ghost {
  cursor: pointer;
  border: 1px solid transparent;
  border-radius: 11px;
  padding: 10px 12px;
  color: #ffffff;
  font-family: "Space Grotesk", sans-serif;
  font-weight: 700;
  background: linear-gradient(135deg, var(--accent), #3e8df0);
  transition: transform .16s ease, filter .16s ease, box-shadow .16s ease;
  text-align: center;
  text-decoration: none;
  display: inline-block;
  box-shadow: 0 8px 18px rgba(15, 106, 214, 0.23);
}

.ghost {
  background: linear-gradient(135deg, #ffffff, #f6f8ff);
  color: #2c3550;
  border-color: #d9deea;
  font-weight: 600;
  box-shadow: none;
}

.compact { padding: 8px 10px; }

button:hover, .ghost:hover {
  transform: translateY(-1px);
  filter: brightness(1.02);
}

button:active, .ghost:active {
  transform: translateY(0);
}

button:disabled {
  opacity: 0.55;
  cursor: not-allowed;
  transform: none;
  filter: grayscale(0.2);
  box-shadow: none;
}

button:focus-visible,
.ghost:focus-visible,
.input:focus-visible,
.switch:focus-visible {
  outline: 2px solid rgba(15, 106, 214, 0.45);
  outline-offset: 2px;
}

.code {
  margin: 0;
  white-space: pre-wrap;
  font: 500 12px/1.45 "IBM Plex Mono", monospace;
  background: #1e2330;
  border: 1px solid #343d52;
  color: #f2f6ff;
  border-radius: 12px;
  padding: 11px;
  min-height: 146px;
}

.code.ok {
  border-color: #2f8e6b;
}

.code.err {
  border-color: #8e3636;
}

.trend {
  margin-top: 10px;
}

.trend-svg {
  width: 100%;
  height: 56px;
  display: block;
  border-radius: 10px;
  border: 1px solid #d7deec;
  background: linear-gradient(180deg, #fbfdff, #f2f6ff);
}

.trend-svg path {
  stroke: #0f6ad6;
  stroke-width: 2;
  fill: none;
  stroke-linecap: round;
  stroke-linejoin: round;
}

.timeline {
  list-style: none;
  margin: 10px 0 0;
  padding: 0;
  display: grid;
  gap: 8px;
  max-height: 180px;
  overflow: auto;
}

.timeline-item {
  border: 1px solid #d8ddea;
  border-left: 4px solid #0f6ad6;
  background: #f9fbff;
  color: #2a3246;
  border-radius: 10px;
  padding: 8px 10px;
  font: 500 12px/1.45 "IBM Plex Mono", monospace;
}

.timeline-item.warn {
  border-left-color: #dc8c00;
}

.timeline-item.err {
  border-left-color: #d64545;
}

.timeline-item.muted {
  color: #6b748a;
  border-left-color: #a8b0c2;
}

.input {
  width: 100%;
  border-radius: 11px;
  border: 1px solid #d7dbe6;
  background: #ffffff;
  color: #222938;
  padding: 10px 11px;
  font: 500 14px/1.45 "IBM Plex Mono", monospace;
  outline: none;
}

.input:focus {
  border-color: #4b8ff0;
  box-shadow: 0 0 0 3px rgba(75, 143, 240, 0.2);
}

.footer {
  margin-top: 18px;
  color: #65708a;
  font-size: 0.89rem;
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
  border: 1px solid #cbd3e4;
  background: #eff2f9;
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
  background: #ffffff;
  box-shadow: 0 1px 4px rgba(0, 0, 0, 0.2);
  transition: transform .2s ease;
}

.switch:checked {
  background: linear-gradient(135deg, #f66e3c, #f39f36);
  border-color: #e97a22;
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

@media (prefers-reduced-motion: reduce) {
  *, *::before, *::after {
    animation: none !important;
    transition: none !important;
    scroll-behavior: auto !important;
  }
}
`

const productUIJS = `
var refreshTimer = null;
var smokeInFlight = false;
var healthLatencyHistory = [];
var readyLatencyHistory = [];
var healthOkHistory = [];
var readyOkHistory = [];
var previousOverallState = '';
var lastRefreshAt = 0;

function setPillState(id, state) {
  var el = document.getElementById(id);
  if (!el) {
    return;
  }
  el.classList.remove('ok', 'warn', 'err');
  if (state) {
    el.classList.add(state);
  }
}

function pushMetric(history, value, maxLen) {
  history.push(value);
  while (history.length > maxLen) {
    history.shift();
  }
}

function renderTrend(svgId, history) {
  var svg = document.getElementById(svgId);
  if (!svg || !history.length) {
    return;
  }
  var path = svg.querySelector('path');
  var width = 260;
  var height = 56;
  var min = Math.min.apply(Math, history);
  var max = Math.max.apply(Math, history);
  var range = Math.max(1, max - min);
  var step = history.length > 1 ? width / (history.length - 1) : width;
  var d = history.map(function(v, i) {
    var x = i * step;
    var y = height - ((v - min) / range) * (height - 8) - 4;
    return (i === 0 ? 'M' : 'L') + x.toFixed(2) + ' ' + y.toFixed(2);
  }).join(' ');
  path.setAttribute('d', d);
}

function addTimelineEvent(message, state) {
  var log = document.getElementById('incidentLog');
  if (!log) {
    return;
  }
  var first = log.querySelector('.timeline-item.muted');
  if (first) {
    first.remove();
  }
  var li = document.createElement('li');
  li.className = 'timeline-item' + (state ? ' ' + state : '');
  li.textContent = '[' + new Date().toLocaleTimeString() + '] ' + message;
  log.prepend(li);
  while (log.children.length > 12) {
    log.removeChild(log.lastChild);
  }
}

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
    // One fast retry smooths transient browser/network hiccups for operator dashboards.
    try {
      var retryRes = await fetch(path, { headers: withAuth ? authHeader() : {} });
      return {
        status: retryRes.status,
        ok: retryRes.ok,
        latency: Math.round(performance.now() - started)
      };
    } catch (_) {
      return {
        status: 'ERR',
        ok: false,
        latency: Math.round(performance.now() - started)
      };
    }
  } finally {
    clearTimeout(timer);
  }
}

function computeSLA(history) {
  if (!history.length) {
    return '--';
  }
  var success = history.filter(function(v) { return Boolean(v); }).length;
  var pct = (success / history.length) * 100;
  return pct.toFixed(0) + '%';
}

function paintSLA(id, history) {
  var text = computeSLA(history);
  var el = document.getElementById(id);
  el.textContent = 'sla(20): ' + text;
  if (text === '--') {
    setPillState(id, 'warn');
    return;
  }
  var value = Number(text.replace('%', ''));
  if (value >= 99) {
    setPillState(id, 'ok');
  } else if (value >= 95) {
    setPillState(id, 'warn');
  } else {
    setPillState(id, 'err');
  }
}

function pushBool(history, value, maxLen) {
  history.push(Boolean(value));
  while (history.length > maxLen) {
    history.shift();
  }
}

async function refreshStatus() {
  updateLastRefresh();
  var health = await check('/healthz', 2200, false);
  var ready = await check('/readyz', 2200, false);

  setStatus('health', health);
  setStatus('ready', ready);
  if (typeof health.latency === 'number') {
    pushMetric(healthLatencyHistory, health.latency, 20);
    renderTrend('healthTrend', healthLatencyHistory);
  }
  if (typeof ready.latency === 'number') {
    pushMetric(readyLatencyHistory, ready.latency, 20);
    renderTrend('readyTrend', readyLatencyHistory);
  }
  pushBool(healthOkHistory, health.ok, 20);
  pushBool(readyOkHistory, ready.ok, 20);
  paintSLA('healthSla', healthOkHistory);
  paintSLA('readySla', readyOkHistory);

  var admin = await check('/admin/configz', 2600, true);
  var adminCode = document.getElementById('adminCode');
  var adminState = document.getElementById('adminState');
  adminCode.textContent = 'status: ' + admin.status;
  setPillState('adminCode', admin.status === 200 ? 'ok' : (admin.status === 401 || admin.status === 403 ? 'warn' : 'err'));
  if (admin.status === 200) {
    adminState.textContent = 'state: visible';
    setPillState('adminState', 'ok');
  } else if (admin.status === 401 || admin.status === 403) {
    adminState.textContent = 'state: locked (auth required)';
    setPillState('adminState', 'warn');
  } else {
    adminState.textContent = 'state: unavailable';
    setPillState('adminState', 'err');
  }

  var overall = document.getElementById('overallState');
  if (health.ok && ready.ok) {
    overall.textContent = 'service: healthy';
    setPillState('overallState', 'ok');
    if (previousOverallState !== 'healthy') {
      addTimelineEvent('Service transitioned to healthy', 'ok');
      previousOverallState = 'healthy';
    }
  } else if (health.status === 'ERR' || ready.status === 'ERR') {
    overall.textContent = 'service: unreachable';
    setPillState('overallState', 'err');
    if (previousOverallState !== 'unreachable') {
      addTimelineEvent('Service became unreachable from browser', 'err');
      previousOverallState = 'unreachable';
    }
  } else {
    overall.textContent = 'service: degraded';
    setPillState('overallState', 'warn');
    if (previousOverallState !== 'degraded') {
      addTimelineEvent('Service transitioned to degraded', 'warn');
      previousOverallState = 'degraded';
    }
  }
}

function setResultState(state) {
  var result = document.getElementById('result');
  result.className = 'code';
  if (state === 'ok') {
    result.classList.add('ok');
  }
  if (state === 'err') {
    result.classList.add('err');
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
    setResultState('err');
    result.textContent = 'Send blocked by safety guard. Enable "webhook test sends" first.';
    return;
  }

  if (smokeInFlight) {
    setResultState('err');
    result.textContent = 'A smoke request is already in progress. Please wait.';
    return;
  }

  var payloadObj;
  try {
    payloadObj = JSON.parse(payload);
  } catch (_) {
    setResultState('err');
    result.textContent = 'Payload must be valid JSON before sending.';
    return;
  }

  smokeInFlight = true;
  setSmokeSendEnabled(false);
  var started = performance.now();
  setResultState('');
  addTimelineEvent('Smoke request started for ' + endpoint, 'warn');
  result.textContent = 'Sending to ' + endpoint + '...';
  try {
    var headers = Object.assign({ 'Content-Type': 'application/json' }, authHeader(), parseExtraHeaders());
    var res = await fetch('/ui/smoke-test', {
      method: 'POST',
      headers: authHeader(),
      body: JSON.stringify({
        endpoint: endpoint,
        payload: JSON.stringify(payloadObj),
        headers: headers
      })
    });
    var body = await res.text();
    var elapsed = Math.round(performance.now() - started);
    var parsed;
    try {
      parsed = JSON.parse(body);
    } catch (_) {
      parsed = { status: res.status, body: body };
    }
    result.textContent = [
      'Endpoint: ' + endpoint,
      'Status: ' + (parsed.status || res.status),
      'Latency: ' + elapsed + 'ms',
      '',
      (parsed.body || body || '(no response body)'),
      '',
      'Note: webhook providers usually require valid signatures/tokens.'
    ].join('\n');
    setResultState(res.ok ? 'ok' : 'err');
    addTimelineEvent('Smoke request completed for ' + endpoint + ' with status ' + (parsed.status || res.status), res.ok ? 'ok' : 'warn');
  } catch (err) {
    setResultState('err');
    result.textContent = 'Request failed: ' + (err && err.message ? err.message : String(err));
    addTimelineEvent('Smoke request failed for ' + endpoint, 'err');
  } finally {
    smokeInFlight = false;
    setSmokeSendEnabled(document.getElementById('allowSend').checked);
  }
}

function setSmokeSendEnabled(enabled) {
  document.querySelectorAll('button[data-endpoint]').forEach(function(btn) {
    btn.disabled = !enabled;
    btn.setAttribute('aria-disabled', enabled ? 'false' : 'true');
  });
  var state = document.getElementById('smokeGuardState');
  state.textContent = enabled ? 'guard: enabled' : 'guard: locked';
  setPillState('smokeGuardState', enabled ? 'ok' : 'warn');
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
  var enabled = document.getElementById('autoRefresh').checked;
  localStorage.setItem('tpb.ui.autoRefresh', enabled ? '1' : '0');
  document.getElementById('refreshMode').textContent = 'auto refresh: ' + (enabled ? 'on' : 'off');
  if (enabled) {
    refreshTimer = setInterval(refreshStatus, 15000);
  }
}

function updateLastRefresh() {
  lastRefreshAt = Date.now();
  document.getElementById('lastRefresh').textContent = 'last refresh: ' + new Date().toLocaleTimeString();
}

function updateFreshnessIndicator() {
  if (!lastRefreshAt) {
    return;
  }
  var ageSec = Math.floor((Date.now() - lastRefreshAt) / 1000);
  var text = 'last refresh: ' + ageSec + 's ago';
  document.getElementById('lastRefresh').textContent = text;
  if (ageSec <= 20) {
    setPillState('lastRefresh', 'ok');
  } else if (ageSec <= 45) {
    setPillState('lastRefresh', 'warn');
  } else {
    setPillState('lastRefresh', 'err');
  }
}

document.querySelectorAll('button[data-endpoint]').forEach(function(btn) {
  btn.addEventListener('click', function() {
    sendSample(btn.getAttribute('data-endpoint'));
  });
});

document.getElementById('manualRefresh').addEventListener('click', refreshStatus);
document.getElementById('autoRefresh').addEventListener('change', setupAutoRefresh);
document.getElementById('allowSend').addEventListener('change', function(ev) {
  setSmokeSendEnabled(Boolean(ev.target.checked));
});
document.getElementById('saveToken').addEventListener('click', function() {
  persistToken();
  refreshStatus();
});
document.getElementById('token').addEventListener('keydown', function(ev) {
  if (ev.key === 'Enter') {
    persistToken();
    refreshStatus();
  }
});
document.getElementById('clearToken').addEventListener('click', function() {
  document.getElementById('token').value = '';
  sessionStorage.removeItem('tpb.ui.jwt');
  refreshStatus();
});

document.addEventListener('keydown', function(ev) {
  var tag = document.activeElement && document.activeElement.tagName ? document.activeElement.tagName.toLowerCase() : '';
  var isTyping = tag === 'input' || tag === 'textarea';
  if (isTyping) {
    return;
  }
  var key = ev.key ? ev.key.toLowerCase() : '';
  if (key === 'r') {
    ev.preventDefault();
    refreshStatus();
    return;
  }
  if (key === 's') {
    ev.preventDefault();
    persistToken();
    refreshStatus();
    return;
  }
  if (key === 'c') {
    ev.preventDefault();
    document.getElementById('token').value = '';
    sessionStorage.removeItem('tpb.ui.jwt');
    refreshStatus();
  }
});

loadToken();
if (localStorage.getItem('tpb.ui.autoRefresh') === '0') {
  document.getElementById('autoRefresh').checked = false;
}
refreshStatus();
setupAutoRefresh();
updateLastRefresh();
setSmokeSendEnabled(document.getElementById('allowSend').checked);
setInterval(updateFreshnessIndicator, 1000);
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

	if err := productUITemplate.Execute(w, struct{ Version string }{Version: uiAssetVersion}); err != nil {
		return
	}
	_ = r // future use: request logging
}

// ProductUIStyles serves versioned CSS for the product UI.
func ProductUIStyles(w http.ResponseWriter, r *http.Request) {
	setUISecurityHeaders(w)
	setAssetCachingHeaders(w)
	if isNotModified(r, w) {
		return
	}
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	if _, err := w.Write([]byte(productUICSS)); err != nil {
		return
	}
}

// ProductUIScript serves versioned JS for the product UI.
func ProductUIScript(w http.ResponseWriter, r *http.Request) {
	setUISecurityHeaders(w)
	setAssetCachingHeaders(w)
	if isNotModified(r, w) {
		return
	}
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	if _, err := w.Write([]byte(productUIJS)); err != nil {
		return
	}
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

func clientIPFromRequestSmoke(remoteAddr string, header http.Header, trustedProxyNets []*net.IPNet) string {
	if len(trustedProxyNets) == 0 {
		return clientIP(remoteAddr)
	}
	remoteHost, _, err := net.SplitHostPort(strings.TrimSpace(remoteAddr))
	if err != nil {
		remoteHost = strings.TrimSpace(remoteAddr)
	}
	remoteIP := net.ParseIP(remoteHost)
	if remoteIP != nil && ipInNetsSmoke(remoteIP, trustedProxyNets) {
		xff := strings.TrimSpace(header.Get("X-Forwarded-For"))
		if xff != "" {
			parts := strings.Split(xff, ",")
			if len(parts) > 0 {
				if ip := strings.TrimSpace(parts[0]); net.ParseIP(ip) != nil {
					return ip
				}
			}
		}
		if xr := strings.TrimSpace(header.Get("X-Real-IP")); net.ParseIP(xr) != nil {
			return xr
		}
	}
	return clientIP(remoteAddr)
}

func parseSmokeProxyCIDRs(cidrs []string) []*net.IPNet {
	if len(cidrs) == 0 {
		return nil
	}
	result := make([]*net.IPNet, 0, len(cidrs))
	for _, cidr := range cidrs {
		_, network, err := net.ParseCIDR(strings.TrimSpace(cidr))
		if err != nil {
			continue
		}
		result = append(result, network)
	}
	return result
}

func ipInNetsSmoke(ip net.IP, nets []*net.IPNet) bool {
	for _, n := range nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

func allowUISmokeRequest(ip string, now time.Time) bool {
	uiSmokeRateLimiter.mu.Lock()
	defer uiSmokeRateLimiter.mu.Unlock()

	if ip == "" {
		ip = "unknown"
	}

	if now.Sub(uiSmokeRateLimiter.lastCleanup) >= time.Minute {
		for key, entry := range uiSmokeRateLimiter.entries {
			if now.Sub(entry.windowStart) >= 2*time.Minute {
				delete(uiSmokeRateLimiter.entries, key)
			}
		}
		uiSmokeRateLimiter.lastCleanup = now
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
// It accepts a wrappedHandler that includes the full middleware chain (rate limiting, auth, etc.)
// to ensure smoke tests accurately represent production traffic patterns.
func NewUISmokeTestProxy(wrappedHandler http.Handler, trustedProxyCIDRs []string) http.HandlerFunc {
	trustedNets := parseSmokeProxyCIDRs(trustedProxyCIDRs)
	return func(w http.ResponseWriter, r *http.Request) {
		setUISecurityHeaders(w)
		w.Header().Set("Cache-Control", "no-store")

		if !allowUISmokeRequest(clientIPFromRequestSmoke(r.RemoteAddr, r.Header, trustedNets), time.Now().UTC()) {
			writeJSON(w, http.StatusTooManyRequests, map[string]string{
				"error": "rate limit exceeded for UI smoke tests",
			})
			return
		}

		bodyReader := io.LimitReader(r.Body, uiSmokeMaxBodyBytes)
		defer func() {
			_ = r.Body.Close()
		}()

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
		wrappedHandler.ServeHTTP(rr, internalReq)

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
