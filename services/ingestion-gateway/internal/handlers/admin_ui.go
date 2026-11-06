package handlers

import (
	"html/template"
	"net/http"
)

const adminUIVersion = "20260517.1"

var adminUITemplate = template.Must(template.New("admin_ui").Parse(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>TeamPulse Bridge | Admin Control Plane</title>
  <link rel="preconnect" href="https://fonts.googleapis.com">
  <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
  <link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&family=JetBrains+Mono:wght@400;500&display=swap" rel="stylesheet">
  <link rel="stylesheet" href="/assets/admin.css?v={{.Version}}" />
</head>
<body class="admin-theme">
  <nav class="sidebar">
    <div class="brand">TPB Admin</div>
    <ul class="nav-links">
      <li class="active"><a href="#dashboard">Dashboard</a></li>
      <li><a href="#failed-events">Failed Events</a></li>
      <li><a href="#security-audit">Security Audit</a></li>
      <li><a href="#config">Configuration</a></li>
    </ul>
    <div class="sidebar-footer">
      <div class="version">{{.Version}}</div>
    </div>
  </nav>

  <main class="content">
    <header class="content-header">
      <h1 id="pageTitle">Admin Dashboard</h1>
      <div class="actions">
        <button id="refreshBtn" class="btn btn-outline">Refresh</button>
      </div>
    </header>

    <div id="dashboard" class="page">
      <section class="stat-grid">
        <div class="stat-card">
          <div class="label">Queue Depth</div>
          <div id="queueDepth" class="value">--</div>
        </div>
        <div class="stat-card">
          <div class="label">Overall Health</div>
          <div id="cbStatus" class="value">--</div>
        </div>
        <div class="stat-card">
          <div class="label">Failure Ratio</div>
          <div id="failureCount" class="value">--</div>
        </div>
      </section>

      <section class="card">
        <div class="section-header">
          <h2>Provider Health</h2>
          <span id="providerHealthSummary" class="section-badge">loading...</span>
        </div>
        <div id="providerHealthGrid" class="provider-grid">
          <div class="empty-row">Loading provider health...</div>
        </div>
      </section>

      <section class="card">
        <h2>Live Security Feed</h2>
        <div class="table-scroll">
          <table id="securityFeedTable">
            <thead>
              <tr>
                <th>Time</th>
                <th>Outcome</th>
                <th>Reason</th>
                <th>Path</th>
                <th>Client IP</th>
              </tr>
            </thead>
            <tbody>
              <tr><td colspan="5">Loading...</td></tr>
            </tbody>
          </table>
        </div>
      </section>
    </div>

    <div id="failed-events" class="page" style="display:none">
      <section class="card">
        <h2>Failed Webhook Explorer</h2>
        <div class="table-scroll">
          <table id="failedEventsTable">
            <thead>
              <tr>
                <th>ID</th>
                <th>Source</th>
                <th>Reason</th>
                <th>Failed At</th>
                <th>Actions</th>
              </tr>
            </thead>
            <tbody>
              <tr><td colspan="5">Loading...</td></tr>
            </tbody>
          </table>
        </div>
      </section>
    </div>
  </main>

  <script src="/assets/admin.js?v={{.Version}}"></script>
</body>
</html>
`))

func (h *AdminHandler) HandleAdminUI(w http.ResponseWriter, r *http.Request) {
	data := struct {
		Version string
	}{
		Version: adminUIVersion,
	}
	if err := adminUITemplate.Execute(w, data); err != nil {
		h.logger.Error("admin ui template execution failed", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}

func (h *AdminHandler) AdminUIStyles(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/css")
	_, _ = w.Write([]byte(`
:root {
  --bg: #0f172a;
  --sidebar: #1e293b;
  --card: #1e293b;
  --text: #f8fafc;
  --text-muted: #94a3b8;
  --primary: #38bdf8;
  --danger: #ef4444;
  --success: #22c55e;
  --border: #334155;
}

body {
  margin: 0;
  font-family: 'Inter', sans-serif;
  background: var(--bg);
  color: var(--text);
  display: flex;
  height: 100vh;
}

.sidebar {
  width: 240px;
  background: var(--sidebar);
  border-right: 1px solid var(--border);
  display: flex;
  flex-direction: column;
}

.brand {
  padding: 2rem;
  font-weight: 700;
  font-size: 1.25rem;
  color: var(--primary);
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.nav-links {
  list-style: none;
  padding: 0;
  margin: 0;
  flex: 1;
}

.nav-links li a {
  display: block;
  padding: 1rem 2rem;
  color: var(--text-muted);
  text-decoration: none;
  transition: all 0.2s;
}

.nav-links li.active a, .nav-links li a:hover {
  background: rgba(56, 189, 248, 0.1);
  color: var(--primary);
  border-left: 4px solid var(--primary);
}

.content {
  flex: 1;
  overflow-y: auto;
  padding: 2rem;
}

.content-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 2rem;
}

.stat-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 1.5rem;
  margin-bottom: 2.5rem;
}

.stat-card {
  background: var(--card);
  padding: 1.5rem;
  border-radius: 0.75rem;
  border: 1px solid var(--border);
}

.stat-card .label {
  color: var(--text-muted);
  font-size: 0.875rem;
  margin-bottom: 0.5rem;
}

.stat-card .value {
  font-size: 1.5rem;
  font-weight: 700;
  font-family: 'JetBrains Mono', monospace;
}

.card {
  background: var(--card);
  border-radius: 0.75rem;
  border: 1px solid var(--border);
  padding: 1.5rem;
}

.section-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 0.75rem;
  margin-bottom: 1rem;
}

.section-badge {
  border-radius: 999px;
  background: rgba(56, 189, 248, 0.08);
  color: var(--primary);
  border: 1px solid rgba(56, 189, 248, 0.3);
  padding: 0.35rem 0.7rem;
  font-size: 0.75rem;
  font-weight: 600;
}

.provider-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: 1rem;
}

.provider-card {
  background: rgba(15, 23, 42, 0.02);
  border: 1px solid var(--border);
  border-radius: 0.75rem;
  padding: 1rem;
}

.provider-card.healthy { border-color: rgba(34, 197, 94, 0.35); }
.provider-card.warning { border-color: rgba(245, 158, 11, 0.35); }
.provider-card.degraded { border-color: rgba(239, 68, 68, 0.35); }

.provider-header {
  display: flex;
  justify-content: space-between;
  gap: 0.5rem;
  align-items: center;
  margin-bottom: 0.75rem;
}

.status-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border-radius: 999px;
  font-size: 0.7rem;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  padding: 0.2rem 0.5rem;
}

.status-badge.healthy { background: rgba(34, 197, 94, 0.12); color: #15803d; }
.status-badge.warning { background: rgba(245, 158, 11, 0.12); color: #b45309; }
.status-badge.degraded { background: rgba(239, 68, 68, 0.12); color: #b91c1c; }

.provider-metrics {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 0.6rem;
  margin-bottom: 0.75rem;
}

.provider-metrics div {
  background: rgba(148, 163, 184, 0.05);
  border-radius: 0.5rem;
  padding: 0.5rem 0.6rem;
}

.provider-metrics small {
  display: block;
  color: var(--text-muted);
  margin-bottom: 0.2rem;
}

.provider-metrics strong {
  font-family: 'JetBrains Mono', monospace;
  font-size: 0.82rem;
}

.provider-card p {
  margin: 0;
  color: var(--text-muted);
  font-size: 0.78rem;
  line-height: 1.5;
}

.empty-row {
  color: var(--text-muted);
  padding: 1rem 0;
}

.table-scroll {
  overflow-x: auto;
}

table {
  width: 100%;
  border-collapse: collapse;
  margin-top: 1rem;
}

th {
  text-align: left;
  color: var(--text-muted);
  font-weight: 500;
  padding: 0.75rem 1rem;
  border-bottom: 1px solid var(--border);
}

td {
  padding: 1rem;
  border-bottom: 1px solid var(--border);
  font-size: 0.875rem;
}

.btn {
  padding: 0.5rem 1rem;
  border-radius: 0.5rem;
  font-weight: 500;
  cursor: pointer;
  transition: opacity 0.2s;
}

.btn-outline {
  background: transparent;
  border: 1px solid var(--primary);
  color: var(--primary);
}
`))
}

func (h *AdminHandler) AdminUIScript(w http.ResponseWriter, r *http.Request) {
	setUISecurityHeaders(w)
	setAssetCachingHeaders(w)
	if isNotModified(r, w) {
		return
	}
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	_, _ = w.Write([]byte(`
async function fetchData(url) {
  try {
    const response = await fetch(url, { headers: { 'Accept': 'application/json' } });
    if (!response.ok) {
      return null;
    }
    return await response.json();
  } catch (err) {
    console.error('Fetch error:', err);
    return null;
  }
}

function normalizeStatus(status) {
  const value = (status || 'unknown').toLowerCase();
  if (value === 'healthy' || value === 'ok') {
    return 'healthy';
  }
  if (value === 'warning' || value === 'warn') {
    return 'warning';
  }
  if (value === 'degraded' || value === 'critical' || value === 'error' || value === 'err') {
    return 'degraded';
  }
  return 'unknown';
}

function renderProviderHealth(summary) {
  const grid = document.getElementById('providerHealthGrid');
  if (!grid) {
    return;
  }
  const providers = summary && Array.isArray(summary.providers) ? summary.providers : [];
  const badge = document.getElementById('providerHealthSummary');
  if (badge) {
    badge.textContent = providers.length ? (summary.summary && summary.summary.healthy !== undefined ? (summary.summary.healthy + ' healthy / ' + providers.length + ' providers') : (providers.length + ' providers')) : 'no provider data';
  }

  if (!providers.length) {
    grid.innerHTML = '<div class="empty-row">No provider health data yet.</div>';
    return;
  }

  grid.innerHTML = '';
  providers.forEach(function(provider) {
    const status = normalizeStatus(provider.status);
    const card = document.createElement('article');
    card.className = 'provider-card ' + status;

    const header = document.createElement('div');
    header.className = 'provider-header';

    const name = document.createElement('strong');
    name.textContent = provider.source || 'unknown';

    const badgeNode = document.createElement('span');
    badgeNode.className = 'status-badge ' + status;
    badgeNode.textContent = status;

    header.appendChild(name);
    header.appendChild(badgeNode);

    const metrics = document.createElement('div');
    metrics.className = 'provider-metrics';

    ['depth', 'capacity', 'usage_ratio', 'failure_ratio'].forEach(function(key) {
      const item = document.createElement('div');
      const label = document.createElement('small');
      label.textContent = key.replace('_', ' ');
      const value = document.createElement('strong');
      const raw = provider[key];
      if (key === 'usage_ratio' || key === 'failure_ratio') {
        const pct = Number(raw || 0) * 100;
        value.textContent = pct.toFixed(0) + '%';
      } else {
        value.textContent = String(raw ?? 0);
      }
      item.appendChild(label);
      item.appendChild(value);
      metrics.appendChild(item);
    });

    const note = document.createElement('p');
    note.textContent = provider.recommendation || 'No recommendation yet.';

    card.appendChild(header);
    card.appendChild(metrics);
    card.appendChild(note);
    grid.appendChild(card);
  });
}

async function refreshDashboard() {
  const [healthSummary, providerSummary, security, failed] = await Promise.all([
    fetchData('/admin/health-summary'),
    fetchData('/admin/providers/health'),
    fetchData('/admin/events/security-audit?limit=10'),
    fetchData('/admin/events/failed?limit=5')
  ]);

  const queue = healthSummary && healthSummary.queue ? healthSummary.queue : {};
  const queueDepth = Number(queue.depth || 0);
  const queueCapacity = Number(queue.capacity || 0);
  const queueText = queueCapacity > 0 ? (queueDepth + '/' + queueCapacity) : String(queueDepth);
  const overallHealth = (healthSummary && healthSummary.status) ? healthSummary.status : 'unknown';
  const failureRatio = Number(queue.failure_ratio || 0) * 100;

  const queueDepthEl = document.getElementById('queueDepth');
  if (queueDepthEl) {
    queueDepthEl.textContent = queueText;
  }

  const healthStateEl = document.getElementById('cbStatus');
  if (healthStateEl) {
    healthStateEl.textContent = overallHealth;
    healthStateEl.classList.remove('healthy', 'warning', 'degraded');
    if (overallHealth === 'healthy') {
      healthStateEl.classList.add('healthy');
    } else if (overallHealth === 'warning') {
      healthStateEl.classList.add('warning');
    } else if (overallHealth === 'degraded') {
      healthStateEl.classList.add('degraded');
    }
  }

  const failureEl = document.getElementById('failureCount');
  if (failureEl) {
    failureEl.textContent = failureRatio.toFixed(1) + '%';
  }

  renderProviderHealth(providerSummary || healthSummary);

  if (security && security.records) {
    const tbody = document.querySelector('#securityFeedTable tbody');
    if (tbody) {
      tbody.innerHTML = security.records.map(function(record) {
        return '<tr><td>' + new Date(record.occurred_at).toLocaleTimeString() + '</td><td><span class="status-badge ' + normalizeStatus(record.outcome) + '">' + (record.outcome || 'unknown') + '</span></td><td>' + (record.reason || '--') + '</td><td>' + (record.path || '--') + '</td><td>' + (record.client_ip || '--') + '</td></tr>';
      }).join('');
    }
  }

  if (failed && failed.events) {
    const tbody = document.querySelector('#failedEventsTable tbody');
    if (tbody) {
      tbody.innerHTML = failed.events.map(function(event) {
        const id = event.event_id || 'unknown';
        return '<tr><td>' + id.substring(0, 8) + '...</td><td>' + (event.source || '--') + '</td><td>' + (event.reason || '--') + '</td><td>' + new Date(event.failed_at || Date.now()).toLocaleString() + '</td><td><button class="btn btn-outline btn-sm">Replay</button></td></tr>';
      }).join('');
    }
  }
}

document.addEventListener('DOMContentLoaded', function() {
  refreshDashboard();
  const refreshButton = document.getElementById('refreshBtn');
  if (refreshButton) {
    refreshButton.addEventListener('click', refreshDashboard);
  }

  window.addEventListener('hashchange', function() {
    const hash = window.location.hash || '#dashboard';
    document.querySelectorAll('.page').forEach(function(page) {
      page.style.display = 'none';
    });
    const activePage = document.querySelector(hash);
    if (activePage) {
      activePage.style.display = 'block';
    }

    document.querySelectorAll('.nav-links li').forEach(function(item) {
      item.classList.remove('active');
    });
    const link = document.querySelector('.nav-links li a[href="' + hash + '"]');
    if (link && link.parentElement) {
      link.parentElement.classList.add('active');
    }

    const title = document.getElementById('pageTitle');
    if (title) {
      const label = hash.substring(1).replace(/[-_]+/g, ' ');
      title.textContent = 'Admin Dashboard';
      if (label && label !== 'dashboard') {
        title.textContent = label.charAt(0).toUpperCase() + label.slice(1);
      }
    }
  });
});
`))
}
