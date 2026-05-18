package handlers

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

.filters-grid {
  margin-top: 10px;
  display: grid;
  gap: 10px;
  grid-template-columns: repeat(auto-fit, minmax(170px, 1fr));
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

.table-wrap {
  margin-top: 12px;
  border: 1px solid #d7dbe6;
  border-radius: 12px;
  overflow: auto;
  background: #ffffff;
}

.events-table {
  width: 100%;
  border-collapse: collapse;
  min-width: 640px;
  font: 500 12px/1.4 "IBM Plex Mono", monospace;
}

.events-table th,
.events-table td {
  padding: 9px 10px;
  border-bottom: 1px solid #eceff6;
  text-align: left;
  vertical-align: top;
}

.events-table th {
  background: #f4f7ff;
  color: #44506a;
  letter-spacing: 0.04em;
  text-transform: uppercase;
  font-size: 11px;
}

.events-table tr:last-child td {
  border-bottom: none;
}

.events-table th.select-col,
.events-table td.select-col {
  width: 42px;
  text-align: center;
}

.events-actions {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
}

.events-actions button {
  padding: 6px 8px;
  font-size: 11px;
}

.empty-row {
  color: #6b748a;
}

.mono {
  font-family: "IBM Plex Mono", monospace;
}

.row-selector {
  width: 16px;
  height: 16px;
  accent-color: var(--accent);
}

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
var failedEventsInFlight = false;
var replayRequestInFlight = false;
var replayAuditInFlight = false;
var failedEventSelection = {};
var failedEventVisibleOrder = [];
var replayAuditCurrentCursor = '';
var replayAuditNextCursor = '';
var replayAuditHasMore = false;
var replayAuditPage = 1;
var replayAuditQueryFingerprint = '';
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
  await refreshFailedEvents(admin.status);
  await refreshReplayAuditList(admin.status);

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

function setFailedReplayResultState(state) {
  var result = document.getElementById('failedReplayResult');
  if (!result) {
    return;
  }
  result.className = 'code';
  if (state === 'ok') {
    result.classList.add('ok');
  }
  if (state === 'err') {
    result.classList.add('err');
  }
}

function setFailedEventsState(text, state) {
  var el = document.getElementById('failedEventsState');
  if (!el) {
    return;
  }
  el.textContent = text;
  setPillState('failedEventsState', state);
}

function setFailedEventsEmpty(message) {
  var tbody = document.getElementById('failedEventsBody');
  if (!tbody) {
    return;
  }
  failedEventVisibleOrder = [];
  pruneFailedEventSelection();
  updateFailedEventSelectionState();
  tbody.innerHTML = '';
  var tr = document.createElement('tr');
  var td = document.createElement('td');
  td.colSpan = 6;
  td.className = 'empty-row';
  td.textContent = message;
  tr.appendChild(td);
  tbody.appendChild(tr);
}

function selectedFailedEventIDs() {
  return failedEventVisibleOrder.filter(function(eventID) {
    return Boolean(failedEventSelection[eventID]);
  });
}

function pruneFailedEventSelection() {
  var allowed = {};
  failedEventVisibleOrder.forEach(function(eventID) {
    allowed[eventID] = true;
  });
  Object.keys(failedEventSelection).forEach(function(eventID) {
    if (!allowed[eventID]) {
      delete failedEventSelection[eventID];
    }
  });
}

function clearFailedEventSelection() {
  failedEventSelection = {};
  updateFailedEventSelectionState();
}

function updateFailedEventSelectionState() {
  var selected = selectedFailedEventIDs();
  var count = selected.length;
  var stateEl = document.getElementById('failedEventSelectionState');
  if (stateEl) {
    stateEl.textContent = 'selected: ' + String(count);
    setPillState('failedEventSelectionState', count > 0 ? 'ok' : '');
  }

  var visibleCount = failedEventVisibleOrder.length;
  var selectAll = document.getElementById('failedEventsSelectAll');
  if (selectAll) {
    selectAll.checked = visibleCount > 0 && count === visibleCount;
    selectAll.indeterminate = count > 0 && count < visibleCount;
    selectAll.disabled = replayRequestInFlight || visibleCount === 0;
  }

  document.querySelectorAll('button[data-replay-action]').forEach(function(button) {
    button.disabled = replayRequestInFlight;
    button.setAttribute('aria-disabled', replayRequestInFlight ? 'true' : 'false');
  });
  document.querySelectorAll('input[data-failed-event-checkbox="row"]').forEach(function(checkbox) {
    checkbox.disabled = replayRequestInFlight;
  });

  var dryRunButton = document.getElementById('failedEventsDryRunSelected');
  if (dryRunButton) {
    dryRunButton.disabled = replayRequestInFlight || count === 0;
    dryRunButton.setAttribute('aria-disabled', dryRunButton.disabled ? 'true' : 'false');
  }
  var replayButton = document.getElementById('failedEventsReplaySelected');
  if (replayButton) {
    replayButton.disabled = replayRequestInFlight || count === 0;
    replayButton.setAttribute('aria-disabled', replayButton.disabled ? 'true' : 'false');
  }
  var clearButton = document.getElementById('failedEventsClearSelection');
  if (clearButton) {
    clearButton.disabled = replayRequestInFlight || count === 0;
    clearButton.setAttribute('aria-disabled', clearButton.disabled ? 'true' : 'false');
  }
}

function setReplayAuditState(text, state) {
  var el = document.getElementById('replayAuditState');
  if (!el) {
    return;
  }
  el.textContent = text;
  setPillState('replayAuditState', state);
}

function setReplayAuditEmpty(message) {
  var tbody = document.getElementById('replayAuditBody');
  if (!tbody) {
    return;
  }
  tbody.innerHTML = '';
  var tr = document.createElement('tr');
  var td = document.createElement('td');
  td.colSpan = 5;
  td.className = 'empty-row';
  td.textContent = message;
  tr.appendChild(td);
  tbody.appendChild(tr);
}

function updateReplayAuditPagingState() {
  var pageEl = document.getElementById('replayAuditPage');
  if (pageEl) {
    pageEl.textContent = 'page: ' + String(replayAuditPage) + (replayAuditHasMore ? ' (more)' : '');
    setPillState('replayAuditPage', replayAuditHasMore ? 'warn' : 'ok');
  }
  var nextButton = document.getElementById('replayAuditNext');
  if (nextButton) {
    var disabled = replayAuditInFlight || !replayAuditHasMore || !replayAuditNextCursor;
    nextButton.disabled = disabled;
    nextButton.setAttribute('aria-disabled', disabled ? 'true' : 'false');
  }
}

function clearReplayAuditForwardPaging() {
  replayAuditHasMore = false;
  replayAuditNextCursor = '';
  updateReplayAuditPagingState();
}

function resetReplayAuditPaging() {
  replayAuditCurrentCursor = '';
  replayAuditNextCursor = '';
  replayAuditHasMore = false;
  replayAuditPage = 1;
  updateReplayAuditPagingState();
}

function replayAuditFiltersFromUI() {
  var actorEl = document.getElementById('replayAuditActor');
  var eventIDEl = document.getElementById('replayAuditEventID');
  var resultEl = document.getElementById('replayAuditResult');
  var sortEl = document.getElementById('replayAuditSort');
  var sortValue = sortEl && sortEl.value ? String(sortEl.value).trim().toLowerCase() : 'desc';
  if (sortValue !== 'asc' && sortValue !== 'desc') {
    sortValue = 'desc';
  }
  return {
    actor: actorEl ? actorEl.value.trim() : '',
    eventID: eventIDEl ? eventIDEl.value.trim() : '',
    result: resultEl ? resultEl.value.trim().toLowerCase() : '',
    sort: sortValue
  };
}

function replayAuditLimitValue() {
  var limitInput = document.getElementById('failedEventsLimit');
  var limitValue = Number(limitInput && limitInput.value ? limitInput.value : '20');
  if (!Number.isFinite(limitValue) || limitValue < 1 || limitValue > 100) {
    return 20;
  }
  return Math.round(limitValue);
}

function replayAuditQueryKey(limitValue, filters) {
  return [
    String(limitValue),
    filters.actor,
    filters.eventID,
    filters.result,
    filters.sort
  ].join('|');
}

function formatFailedAt(raw) {
  if (!raw) {
    return '--';
  }
  var parsed = new Date(raw);
  if (Number.isNaN(parsed.getTime())) {
    return String(raw);
  }
  return parsed.toLocaleString();
}

function formatReplayResult(record) {
  if (record && record.result) {
    if (record.error_code) {
      return record.result + ' (' + record.error_code + ')';
    }
    return record.result;
  }
  return '--';
}

function renderFailedEvents(events) {
  var tbody = document.getElementById('failedEventsBody');
  if (!tbody) {
    return;
  }
  tbody.innerHTML = '';
  failedEventVisibleOrder = [];
  if (!events || !events.length) {
    pruneFailedEventSelection();
    updateFailedEventSelectionState();
    setFailedEventsEmpty('No failed events recorded.');
    return;
  }

  events.forEach(function(event) {
    var tr = document.createElement('tr');
    var eventIDValue = event.event_id || '';
    failedEventVisibleOrder.push(eventIDValue);

    var selectCell = document.createElement('td');
    selectCell.className = 'select-col';
    var checkbox = document.createElement('input');
    checkbox.className = 'row-selector';
    checkbox.type = 'checkbox';
    checkbox.setAttribute('data-failed-event-checkbox', 'row');
    checkbox.checked = Boolean(failedEventSelection[eventIDValue]);
    checkbox.disabled = replayRequestInFlight;
    checkbox.setAttribute('aria-label', 'Select failed event ' + (eventIDValue || '--'));
    checkbox.addEventListener('change', function(ev) {
      if (ev.target.checked) {
        failedEventSelection[eventIDValue] = true;
      } else {
        delete failedEventSelection[eventIDValue];
      }
      updateFailedEventSelectionState();
    });
    selectCell.appendChild(checkbox);
    tr.appendChild(selectCell);

    var eventId = document.createElement('td');
    eventId.className = 'mono';
    eventId.textContent = eventIDValue || '--';
    tr.appendChild(eventId);

    var source = document.createElement('td');
    source.textContent = event.source || '--';
    tr.appendChild(source);

    var reason = document.createElement('td');
    reason.className = 'mono';
    reason.textContent = event.reason || '--';
    tr.appendChild(reason);

    var failedAt = document.createElement('td');
    failedAt.textContent = formatFailedAt(event.failed_at);
    tr.appendChild(failedAt);

    var actions = document.createElement('td');
    var actionWrap = document.createElement('div');
    actionWrap.className = 'events-actions';

    var dryRunButton = document.createElement('button');
    dryRunButton.className = 'ghost compact';
    dryRunButton.setAttribute('data-replay-action', 'row');
    dryRunButton.textContent = 'Dry Run';
    dryRunButton.type = 'button';
    dryRunButton.disabled = replayRequestInFlight;
    dryRunButton.addEventListener('click', function() {
      replayFailedEvent(eventIDValue, true);
    });
    actionWrap.appendChild(dryRunButton);

    var replayButton = document.createElement('button');
    replayButton.className = 'compact';
    replayButton.setAttribute('data-replay-action', 'row');
    replayButton.textContent = 'Replay';
    replayButton.type = 'button';
    replayButton.disabled = replayRequestInFlight;
    replayButton.addEventListener('click', function() {
      replayFailedEvent(eventIDValue, false);
    });
    actionWrap.appendChild(replayButton);

    actions.appendChild(actionWrap);
    tr.appendChild(actions);
    tbody.appendChild(tr);
  });

  pruneFailedEventSelection();
  updateFailedEventSelectionState();
}

async function replayFailedEvent(eventID, dryRun) {
  var result = document.getElementById('failedReplayResult');
  if (!result) {
    return;
  }
  if (replayRequestInFlight) {
    setFailedReplayResultState('err');
    result.textContent = 'Another replay request is already in progress. Please wait.';
    return;
  }
  replayRequestInFlight = true;
  updateFailedEventSelectionState();
  setFailedReplayResultState('');
  result.textContent = (dryRun ? 'Validating' : 'Replaying') + ' ' + eventID + '...';
  try {
    var res = await fetch('/admin/events/replay', {
      method: 'POST',
      headers: Object.assign({ 'Content-Type': 'application/json' }, authHeader()),
      body: JSON.stringify({
        event_id: eventID,
        dry_run: Boolean(dryRun)
      })
    });
    var body = await res.text();
    var parsed;
    try {
      parsed = JSON.parse(body);
    } catch (_) {
      parsed = { raw: body };
    }
    var lines = [
      'Event ID: ' + eventID,
      'HTTP: ' + res.status,
      'Mode: ' + (dryRun ? 'dry-run' : 'publish'),
      ''
    ];
    if (parsed && parsed.error && parsed.error.code) {
      lines.push('Error Code: ' + parsed.error.code);
      lines.push('Error: ' + (parsed.error.message || 'request failed'));
    } else {
      lines.push(JSON.stringify(parsed, null, 2));
    }
    result.textContent = lines.join('\n');
    setFailedReplayResultState(res.ok ? 'ok' : 'err');
    addTimelineEvent((dryRun ? 'Dry-run' : 'Replay') + ' for ' + eventID + ' returned ' + res.status, res.ok ? 'ok' : 'warn');
    if (res.ok && !dryRun) {
      refreshFailedEvents();
    }
    refreshReplayAuditList();
  } catch (err) {
    setFailedReplayResultState('err');
    result.textContent = 'Replay request failed: ' + (err && err.message ? err.message : String(err));
    addTimelineEvent('Replay request failed for ' + eventID, 'err');
  } finally {
    replayRequestInFlight = false;
    updateFailedEventSelectionState();
  }
}

async function replayFailedEventsBatch(eventIDs, dryRun) {
  var result = document.getElementById('failedReplayResult');
  if (!result) {
    return;
  }
  if (replayRequestInFlight) {
    setFailedReplayResultState('err');
    result.textContent = 'Another replay request is already in progress. Please wait.';
    return;
  }
  if (!eventIDs || !eventIDs.length) {
    setFailedReplayResultState('err');
    result.textContent = 'Select at least one failed event before running a batch action.';
    return;
  }

  replayRequestInFlight = true;
  updateFailedEventSelectionState();
  setFailedReplayResultState('');
  result.textContent = (dryRun ? 'Validating' : 'Replaying') + ' ' + String(eventIDs.length) + ' selected failed events...';
  try {
    var res = await fetch('/admin/events/replay/batch', {
      method: 'POST',
      headers: Object.assign({ 'Content-Type': 'application/json' }, authHeader()),
      body: JSON.stringify({
        event_ids: eventIDs,
        dry_run: Boolean(dryRun)
      })
    });
    var body = await res.text();
    var parsed;
    try {
      parsed = JSON.parse(body);
    } catch (_) {
      parsed = { raw: body };
    }

    var summary = parsed && parsed.summary ? parsed.summary : {};
    var lines = [
      'Selected: ' + String(eventIDs.length),
      'HTTP: ' + res.status,
      'Mode: ' + (dryRun ? 'dry-run' : 'publish'),
      ''
    ];
    if (parsed && parsed.error && parsed.error.code) {
      lines.push('Error Code: ' + parsed.error.code);
      lines.push('Error: ' + (parsed.error.message || 'request failed'));
    } else {
      lines.push('Status: ' + (parsed.status || '--'));
      lines.push('Requested: ' + String(summary.requested || 0));
      lines.push('Processed: ' + String(summary.processed || 0));
      lines.push('Succeeded: ' + String(summary.succeeded || 0));
      lines.push('Validated: ' + String(summary.validated || 0));
      lines.push('Accepted: ' + String(summary.accepted || 0));
      lines.push('Failed: ' + String(summary.failed || 0));
      if (parsed && parsed.results && parsed.results.length) {
        lines.push('');
        lines.push('Items:');
        parsed.results.forEach(function(item) {
          var line = '- ' + (item.event_id || '--') + ' -> ' + (item.status || 'unknown') + ' (HTTP ' + String(item.http_status || '--') + ')';
          if (item.error_code) {
            line += ' [' + item.error_code + ']';
          }
          lines.push(line);
        });
      }
    }

    result.textContent = lines.join('\n');
    var hasFailures = Boolean(summary.failed);
    setFailedReplayResultState(res.ok && !hasFailures ? 'ok' : 'err');
    addTimelineEvent((dryRun ? 'Batch dry-run' : 'Batch replay') + ' for ' + String(eventIDs.length) + ' events returned ' + res.status, hasFailures ? 'warn' : 'ok');
    if (!dryRun) {
      clearFailedEventSelection();
      refreshFailedEvents();
    }
    refreshReplayAuditList();
  } catch (err) {
    setFailedReplayResultState('err');
    result.textContent = 'Batch replay request failed: ' + (err && err.message ? err.message : String(err));
    addTimelineEvent('Batch replay request failed for ' + String(eventIDs.length) + ' events', 'err');
  } finally {
    replayRequestInFlight = false;
    updateFailedEventSelectionState();
  }
}

async function refreshFailedEvents(adminStatus) {
  if (failedEventsInFlight) {
    return;
  }
  if (adminStatus === 401 || adminStatus === 403) {
    setFailedEventsState('state: locked (auth required)', 'warn');
    setFailedEventsEmpty('Admin token required to load failed events.');
    return;
  }

  var limitInput = document.getElementById('failedEventsLimit');
  var limitValue = Number(limitInput && limitInput.value ? limitInput.value : '20');
  if (!Number.isFinite(limitValue) || limitValue < 1 || limitValue > 100) {
    limitValue = 20;
  }

  failedEventsInFlight = true;
  setFailedEventsState('state: loading', 'warn');
  try {
    var res = await fetch('/admin/events/failed?limit=' + encodeURIComponent(String(limitValue)), {
      headers: authHeader()
    });
    var body = await res.text();
    var parsed;
    try {
      parsed = JSON.parse(body);
    } catch (_) {
      parsed = {};
    }

    if (res.status === 401 || res.status === 403) {
      setFailedEventsState('state: locked (auth required)', 'warn');
      setFailedEventsEmpty('Admin token required to load failed events.');
      return;
    }
    if (!res.ok) {
      setFailedEventsState('state: unavailable', 'err');
      setFailedEventsEmpty('Failed to load failed events (status ' + res.status + ').');
      return;
    }
    if (!parsed.enabled) {
      setFailedEventsState('state: disabled', 'warn');
      setFailedEventsEmpty('Failed-event store is disabled in this environment.');
      return;
    }
    setFailedEventsState('state: ready', 'ok');
    renderFailedEvents(parsed.events || []);
  } catch (_) {
    setFailedEventsState('state: unavailable', 'err');
    setFailedEventsEmpty('Failed to load failed events due to network or browser error.');
  } finally {
    failedEventsInFlight = false;
  }
}

function renderReplayAudit(records) {
  var tbody = document.getElementById('replayAuditBody');
  if (!tbody) {
    return;
  }
  tbody.innerHTML = '';
  if (!records || !records.length) {
    setReplayAuditEmpty('No replay audit records yet.');
    return;
  }

  records.forEach(function(record) {
    var tr = document.createElement('tr');

    var replayedAt = document.createElement('td');
    replayedAt.textContent = formatFailedAt(record.replayed_at);
    tr.appendChild(replayedAt);

    var actor = document.createElement('td');
    actor.className = 'mono';
    actor.textContent = record.actor || '--';
    tr.appendChild(actor);

    var eventID = document.createElement('td');
    eventID.className = 'mono';
    eventID.textContent = record.event_id || '--';
    tr.appendChild(eventID);

    var mode = document.createElement('td');
    mode.className = 'mono';
    mode.textContent = record.mode || '--';
    tr.appendChild(mode);

    var result = document.createElement('td');
    result.className = 'mono';
    result.textContent = formatReplayResult(record);
    tr.appendChild(result);

    tbody.appendChild(tr);
  });
}

async function refreshReplayAuditList(adminStatus, options) {
  options = options || {};
  if (replayAuditInFlight) {
    return;
  }
  if (adminStatus === 401 || adminStatus === 403) {
    setReplayAuditState('state: locked (auth required)', 'warn');
    setReplayAuditEmpty('Admin token required to load replay audit history.');
    resetReplayAuditPaging();
    return;
  }

  var filters = replayAuditFiltersFromUI();
  var limitValue = replayAuditLimitValue();
  var queryKey = replayAuditQueryKey(limitValue, filters);
  var needsReset = Boolean(options.reset) || queryKey !== replayAuditQueryFingerprint;
  if (needsReset) {
    replayAuditQueryFingerprint = queryKey;
    replayAuditCurrentCursor = '';
    replayAuditNextCursor = '';
    replayAuditHasMore = false;
    replayAuditPage = 1;
  }

  var cursor = replayAuditCurrentCursor;
  var page = replayAuditPage;
  if (options.nextPage) {
    if (!replayAuditHasMore || !replayAuditNextCursor) {
      updateReplayAuditPagingState();
      return;
    }
    cursor = replayAuditNextCursor;
    page = replayAuditPage + 1;
  }

  replayAuditInFlight = true;
  updateReplayAuditPagingState();
  setReplayAuditState('state: loading', 'warn');
  try {
    var params = new URLSearchParams();
    params.set('limit', String(limitValue));
    params.set('sort', filters.sort);
    if (filters.actor) {
      params.set('actor', filters.actor);
    }
    if (filters.eventID) {
      params.set('event_id', filters.eventID);
    }
    if (filters.result) {
      params.set('result', filters.result);
    }
    if (cursor) {
      params.set('cursor', cursor);
    }

    var res = await fetch('/admin/events/replay-audit?' + params.toString(), {
      headers: authHeader()
    });
    var body = await res.text();
    var parsed;
    try {
      parsed = JSON.parse(body);
    } catch (_) {
      parsed = {};
    }

    if (res.status === 401 || res.status === 403) {
      setReplayAuditState('state: locked (auth required)', 'warn');
      setReplayAuditEmpty('Admin token required to load replay audit history.');
      resetReplayAuditPaging();
      return;
    }
    if (!res.ok) {
      setReplayAuditState('state: unavailable', 'err');
      clearReplayAuditForwardPaging();
      setReplayAuditEmpty('Failed to load replay audit history (status ' + res.status + ').');
      return;
    }
    if (!parsed.enabled) {
      setReplayAuditState('state: disabled', 'warn');
      setReplayAuditEmpty('Replay audit history is disabled in this environment.');
      resetReplayAuditPaging();
      return;
    }
    replayAuditCurrentCursor = cursor;
    replayAuditPage = page;
    var pageInfo = parsed.page || {};
    replayAuditHasMore = Boolean(pageInfo.has_more);
    replayAuditNextCursor = replayAuditHasMore && pageInfo.next_cursor ? String(pageInfo.next_cursor) : '';

    setReplayAuditState('state: ready', 'ok');
    renderReplayAudit(parsed.records || []);
  } catch (_) {
    setReplayAuditState('state: unavailable', 'err');
    clearReplayAuditForwardPaging();
    setReplayAuditEmpty('Failed to load replay audit history due to network or browser error.');
  } finally {
    replayAuditInFlight = false;
    updateReplayAuditPagingState();
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
document.getElementById('refreshFailedEvents').addEventListener('click', function() {
  refreshFailedEvents();
});
document.getElementById('failedEventsSelectAll').addEventListener('change', function(ev) {
  if (ev.target.checked) {
    failedEventVisibleOrder.forEach(function(eventID) {
      failedEventSelection[eventID] = true;
    });
  } else {
    failedEventVisibleOrder.forEach(function(eventID) {
      delete failedEventSelection[eventID];
    });
  }
  updateFailedEventSelectionState();
});
document.getElementById('failedEventsDryRunSelected').addEventListener('click', function() {
  replayFailedEventsBatch(selectedFailedEventIDs(), true);
});
document.getElementById('failedEventsReplaySelected').addEventListener('click', function() {
  replayFailedEventsBatch(selectedFailedEventIDs(), false);
});
document.getElementById('failedEventsClearSelection').addEventListener('click', function() {
  clearFailedEventSelection();
});
document.getElementById('refreshReplayAudit').addEventListener('click', function() {
  refreshReplayAuditList();
});
document.getElementById('replayAuditNext').addEventListener('click', function() {
  refreshReplayAuditList(undefined, { nextPage: true });
});
document.getElementById('replayAuditReset').addEventListener('click', function() {
  document.getElementById('replayAuditActor').value = '';
  document.getElementById('replayAuditEventID').value = '';
  document.getElementById('replayAuditResult').value = '';
  document.getElementById('replayAuditSort').value = 'desc';
  refreshReplayAuditList(undefined, { reset: true });
});
document.getElementById('failedEventsLimit').addEventListener('change', function() {
  refreshFailedEvents();
  refreshReplayAuditList(undefined, { reset: true });
});
document.getElementById('replayAuditActor').addEventListener('keydown', function(ev) {
  if (ev.key === 'Enter') {
    refreshReplayAuditList(undefined, { reset: true });
  }
});
document.getElementById('replayAuditEventID').addEventListener('keydown', function(ev) {
  if (ev.key === 'Enter') {
    refreshReplayAuditList(undefined, { reset: true });
  }
});
document.getElementById('replayAuditResult').addEventListener('change', function() {
  refreshReplayAuditList(undefined, { reset: true });
});
document.getElementById('replayAuditSort').addEventListener('change', function() {
  refreshReplayAuditList(undefined, { reset: true });
});
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
updateFailedEventSelectionState();
updateReplayAuditPagingState();
setInterval(updateFreshnessIndicator, 1000);
`
