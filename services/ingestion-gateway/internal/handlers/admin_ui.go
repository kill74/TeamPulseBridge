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
          <div class="label">Circuit Breaker</div>
          <div id="cbStatus" class="value">--</div>
        </div>
        <div class="stat-card">
          <div class="label">Recent Failures</div>
          <div id="failureCount" class="value">--</div>
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

func (h *AdminHandler) AdminUIScript(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/javascript")
	_, _ = w.Write([]byte(`
async function fetchData(url) {
  try {
    const response = await fetch(url);
    return await response.json();
  } catch (err) {
    console.error('Fetch error:', err);
    return null;
  }
}

async function refreshDashboard() {
  const security = await fetchData('/admin/events/security-audit?limit=10');
  const failed = await fetchData('/admin/events/failed?limit=5');
  
  if (security && security.records) {
    const tbody = document.querySelector('#securityFeedTable tbody');
    tbody.innerHTML = security.records.map(r => ` + "`" + `
      <tr>
        <td>${new Date(r.occurred_at).toLocaleTimeString()}</td>
        <td><span class="pill ${r.outcome === 'accepted' ? 'success' : 'danger'}">${r.outcome}</span></td>
        <td>${r.reason}</td>
        <td>${r.path}</td>
        <td>${r.client_ip}</td>
      </tr>
    ` + "`" + `).join('');
  }

  if (failed && failed.events) {
    const tbody = document.querySelector('#failedEventsTable tbody');
    tbody.innerHTML = failed.events.map(e => ` + "`" + `
      <tr>
        <td>${e.event_id.substring(0, 8)}...</td>
        <td>${e.source}</td>
        <td>${e.reason}</td>
        <td>${new Date(e.failed_at).toLocaleString()}</td>
        <td><button class="btn btn-outline btn-sm">Replay</button></td>
      </tr>
    ` + "`" + `).join('');
  }
}

document.addEventListener('DOMContentLoaded', () => {
  refreshDashboard();
  document.querySelector('#refreshBtn').addEventListener('click', refreshDashboard);
  
  // Basic routing
  window.addEventListener('hashchange', () => {
    const hash = window.location.hash || '#dashboard';
    document.querySelectorAll('.page').forEach(p => p.style.display = 'none');
    const activePage = document.querySelector(hash);
    if (activePage) activePage.style.display = 'block';
    
    document.querySelectorAll('.nav-links li').forEach(li => li.classList.remove('active'));
    document.querySelector(` + "`" + `.nav-links li a[href="${hash}"]` + "`" + `).parentElement.classList.add('active');
    
    document.querySelector('#pageTitle').textContent = hash.substring(1).charAt(0).toUpperCase() + hash.substring(2);
  });
});
`))
}
