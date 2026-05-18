package handlers

import (
	"html/template"
)

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

      <article class="card span-8 delay-5">
        <h2>Failed Event Explorer</h2>
        <p class="small">Review recent failed publish events, select multiple entries, and replay them without leaving the console. Requires admin access when JWT guard is enabled.</p>
        <div class="row row-top-gap">
          <button id="refreshFailedEvents" class="ghost compact">Refresh Failed Events</button>
          <span id="failedEventsState" class="pill">state: checking</span>
          <span id="failedEventSelectionState" class="pill">selected: 0</span>
        </div>
        <div class="field">
          <label class="label" for="failedEventsLimit">List Limit</label>
          <input id="failedEventsLimit" class="input" type="number" min="1" max="100" value="20" />
        </div>
        <div class="row row-top-gap">
          <button id="failedEventsDryRunSelected" class="ghost compact" type="button">Dry Run Selected</button>
          <button id="failedEventsReplaySelected" class="compact" type="button">Replay Selected</button>
          <button id="failedEventsClearSelection" class="ghost compact" type="button">Clear Selection</button>
        </div>
        <div class="table-wrap">
          <table class="events-table" aria-label="Failed events">
            <thead>
              <tr>
                <th class="select-col"><input id="failedEventsSelectAll" class="row-selector" type="checkbox" aria-label="Select all failed events" data-failed-event-checkbox="header" /></th>
                <th>Event ID</th>
                <th>Source</th>
                <th>Reason</th>
                <th>Failed At</th>
                <th>Actions</th>
              </tr>
            </thead>
            <tbody id="failedEventsBody">
              <tr><td colspan="6" class="empty-row">Loading failed events...</td></tr>
            </tbody>
          </table>
        </div>
        <div class="field">
          <label class="label" for="failedReplayResult">Replay Result</label>
          <pre id="failedReplayResult" class="code" role="status" aria-live="polite">Ready.</pre>
        </div>

        <h2 class="stack-heading">Replay Audit History</h2>
        <div class="row row-top-gap">
          <button id="refreshReplayAudit" class="ghost compact">Refresh Replay Audit</button>
          <button id="replayAuditNext" class="ghost compact" type="button">Next Page</button>
          <button id="replayAuditReset" class="ghost compact" type="button">Reset Filters</button>
          <span id="replayAuditState" class="pill">state: checking</span>
          <span id="replayAuditPage" class="pill">page: 1</span>
        </div>
        <div class="filters-grid">
          <div class="field">
            <label class="label" for="replayAuditActor">Actor</label>
            <input id="replayAuditActor" class="input" type="text" placeholder="dev@example.com" />
          </div>
          <div class="field">
            <label class="label" for="replayAuditEventID">Event ID</label>
            <input id="replayAuditEventID" class="input" type="text" placeholder="evt_1234" />
          </div>
          <div class="field">
            <label class="label" for="replayAuditResult">Result</label>
            <select id="replayAuditResult" class="input">
              <option value="">All</option>
              <option value="accepted">accepted</option>
              <option value="validated">validated</option>
              <option value="failed">failed</option>
            </select>
          </div>
          <div class="field">
            <label class="label" for="replayAuditSort">Sort</label>
            <select id="replayAuditSort" class="input">
              <option value="desc">Newest first</option>
              <option value="asc">Oldest first</option>
            </select>
          </div>
        </div>
        <div class="table-wrap">
          <table class="events-table" aria-label="Replay audit">
            <thead>
              <tr>
                <th>Replayed At</th>
                <th>Actor</th>
                <th>Event ID</th>
                <th>Mode</th>
                <th>Result</th>
              </tr>
            </thead>
            <tbody id="replayAuditBody">
              <tr><td colspan="5" class="empty-row">Loading replay audit history...</td></tr>
            </tbody>
          </table>
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
          <a class="ghost" href="/admin/events/failed" target="_blank" rel="noreferrer">Open Failed Events API</a>
          <a class="ghost" href="/admin/events/replay-audit" target="_blank" rel="noreferrer">Open Replay Audit API</a>
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
