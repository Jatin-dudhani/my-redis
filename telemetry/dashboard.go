package telemetry

const DashboardHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>My-Redis | Real-Time Engine Dashboard</title>
  <style>
    :root {
      --bg: #090d16;
      --card-bg: rgba(18, 24, 38, 0.7);
      --card-border: rgba(255, 255, 255, 0.08);
      --card-hover: rgba(255, 255, 255, 0.12);
      --text-main: #f1f5f9;
      --text-muted: #94a3b8;
      --accent-cyan: #06b6d4;
      --accent-blue: #3b82f6;
      --accent-green: #10b981;
      --accent-purple: #8b5cf6;
      --accent-rose: #f43f5e;
      --glow-cyan: 0 0 25px rgba(6, 182, 212, 0.35);
      --font-stack: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif;
    }
    * { box-sizing: border-box; margin: 0; padding: 0; }
    body {
      background: radial-gradient(circle at 15% 15%, #0f172a 0%, var(--bg) 60%);
      color: var(--text-main);
      font-family: var(--font-stack);
      min-height: 100vh;
      display: flex;
      flex-direction: column;
      overflow-x: hidden;
    }
    header {
      background: rgba(10, 15, 28, 0.85);
      backdrop-filter: blur(16px);
      border-bottom: 1px solid var(--card-border);
      padding: 1rem 2rem;
      display: flex;
      justify-content: space-between;
      align-items: center;
      position: sticky;
      top: 0;
      z-index: 100;
    }
    .brand {
      display: flex;
      align-items: center;
      gap: 0.75rem;
    }
    .brand-logo {
      width: 32px;
      height: 32px;
      background: linear-gradient(135deg, var(--accent-cyan), var(--accent-blue));
      border-radius: 8px;
      display: flex;
      align-items: center;
      justify-content: center;
      font-weight: 800;
      color: #fff;
      font-size: 1.1rem;
      box-shadow: var(--glow-cyan);
    }
    .brand-title {
      font-size: 1.25rem;
      font-weight: 700;
      letter-spacing: -0.5px;
      background: linear-gradient(90deg, #fff, #cbd5e1);
      -webkit-background-clip: text;
      -webkit-text-fill-color: transparent;
    }
    .header-badges {
      display: flex;
      gap: 1rem;
      align-items: center;
    }
    .badge {
      display: inline-flex;
      align-items: center;
      gap: 0.4rem;
      padding: 0.35rem 0.8rem;
      border-radius: 9999px;
      font-size: 0.8rem;
      font-weight: 600;
      background: rgba(16, 185, 129, 0.12);
      border: 1px solid rgba(16, 185, 129, 0.25);
      color: var(--accent-green);
    }
    .pulse-dot {
      width: 8px;
      height: 8px;
      border-radius: 50%;
      background: var(--accent-green);
      box-shadow: 0 0 10px var(--accent-green);
      animation: pulse 2s infinite;
    }
    @keyframes pulse {
      0%, 100% { opacity: 1; transform: scale(1); }
      50% { opacity: 0.4; transform: scale(0.85); }
    }
    main {
      flex: 1;
      padding: 2rem;
      max-width: 1400px;
      margin: 0 auto;
      width: 100%;
      display: flex;
      flex-direction: column;
      gap: 2rem;
    }
    .grid-stats {
      display: grid;
      grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
      gap: 1.25rem;
    }
    .stat-card {
      background: var(--card-bg);
      backdrop-filter: blur(12px);
      border: 1px solid var(--card-border);
      border-radius: 14px;
      padding: 1.25rem 1.5rem;
      display: flex;
      flex-direction: column;
      gap: 0.5rem;
      transition: all 0.2s ease;
    }
    .stat-card:hover {
      border-color: var(--card-hover);
      transform: translateY(-2px);
    }
    .stat-label {
      font-size: 0.85rem;
      color: var(--text-muted);
      font-weight: 500;
      text-transform: uppercase;
      letter-spacing: 0.5px;
    }
    .stat-value {
      font-size: 1.9rem;
      font-weight: 800;
      letter-spacing: -0.5px;
      color: #fff;
    }
    .stat-sub {
      font-size: 0.75rem;
      color: var(--text-muted);
    }
    .section-row {
      display: grid;
      grid-template-columns: 1fr 1fr;
      gap: 1.5rem;
    }
    @media (max-width: 900px) {
      .section-row { grid-template-columns: 1fr; }
    }
    .panel {
      background: var(--card-bg);
      backdrop-filter: blur(12px);
      border: 1px solid var(--card-border);
      border-radius: 14px;
      display: flex;
      flex-direction: column;
      overflow: hidden;
    }
    .panel-header {
      padding: 1rem 1.5rem;
      border-bottom: 1px solid var(--card-border);
      display: flex;
      justify-content: space-between;
      align-items: center;
      background: rgba(255, 255, 255, 0.02);
    }
    .panel-title {
      font-size: 1rem;
      font-weight: 700;
      color: #fff;
    }
    .terminal-output {
      padding: 1.25rem;
      background: #060911;
      font-family: 'SFMono-Regular', Consolas, 'Liberation Mono', Menlo, monospace;
      font-size: 0.88rem;
      height: 280px;
      overflow-y: auto;
      color: #38bdf8;
      line-height: 1.6;
    }
    .terminal-input-bar {
      display: flex;
      border-top: 1px solid var(--card-border);
      background: #0a0e1a;
    }
    .terminal-input-bar input {
      flex: 1;
      padding: 0.85rem 1.25rem;
      background: transparent;
      border: none;
      outline: none;
      color: #fff;
      font-family: monospace;
      font-size: 0.9rem;
    }
    .terminal-input-bar button {
      padding: 0.85rem 1.5rem;
      background: linear-gradient(135deg, var(--accent-cyan), var(--accent-blue));
      border: none;
      color: #fff;
      font-weight: 600;
      cursor: pointer;
      transition: opacity 0.2s;
    }
    .terminal-input-bar button:hover { opacity: 0.9; }
    .key-table-container {
      height: 280px;
      overflow-y: auto;
    }
    table {
      width: 100%;
      border-collapse: collapse;
      font-size: 0.88rem;
    }
    th {
      text-align: left;
      padding: 0.75rem 1.25rem;
      color: var(--text-muted);
      border-bottom: 1px solid var(--card-border);
      font-weight: 600;
      background: rgba(255, 255, 255, 0.01);
    }
    td {
      padding: 0.75rem 1.25rem;
      border-bottom: 1px solid rgba(255, 255, 255, 0.04);
      color: var(--text-main);
    }
    .type-pill {
      display: inline-block;
      padding: 0.2rem 0.6rem;
      border-radius: 6px;
      font-size: 0.75rem;
      font-weight: 700;
      text-transform: uppercase;
    }
    .type-string { background: rgba(59, 130, 246, 0.18); color: #60a5fa; }
    .type-zset   { background: rgba(16, 185, 129, 0.18); color: #34d399; }
    .type-hash   { background: rgba(139, 92, 246, 0.18); color: #a78bfa; }
    .type-list   { background: rgba(245, 158, 11, 0.18); color: #fbbf24; }
    .type-set    { background: rgba(244, 63, 94, 0.18);  color: #fb7185; }
    .type-stream { background: rgba(6, 182, 212, 0.18);  color: #22d3ee; }
  </style>
</head>
<body>
  <header>
    <div class="brand">
      <div class="brand-logo">R</div>
      <div class="brand-title">My-Redis Distributed Engine</div>
    </div>
    <div class="header-badges">
      <span class="badge" id="cluster-badge">
        <span class="pulse-dot"></span>
        <span id="cluster-mode">High-Scale Active</span>
      </span>
      <span style="font-size: 0.85rem; color: var(--text-muted);" id="uptime-display">Uptime: 0s</span>
    </div>
  </header>

  <main>
    <div class="grid-stats">
      <div class="stat-card">
        <div class="stat-label">Throughput</div>
        <div class="stat-value" id="val-ops" style="color: var(--accent-cyan);">0.0</div>
        <div class="stat-sub">Operations / second</div>
      </div>
      <div class="stat-card">
        <div class="stat-label">Total Keys</div>
        <div class="stat-value" id="val-keys" style="color: var(--accent-green);">0</div>
        <div class="stat-sub">256-shard keyspace</div>
      </div>
      <div class="stat-card">
        <div class="stat-label">Memory Used</div>
        <div class="stat-value" id="val-mem" style="color: var(--accent-purple);">0.0 MB</div>
        <div class="stat-sub">Heap runtime alloc</div>
      </div>
      <div class="stat-card">
        <div class="stat-label">Cache Hit Rate</div>
        <div class="stat-value" id="val-hitrate" style="color: #60a5fa;">100%</div>
        <div class="stat-sub" id="val-hitdetails">0 hits / 0 misses</div>
      </div>
      <div class="stat-card">
        <div class="stat-label">Active Clients</div>
        <div class="stat-value" id="val-clients" style="color: #fbbf24;">1</div>
        <div class="stat-sub">TCP connections</div>
      </div>
      <div class="stat-card">
        <div class="stat-label">Commands Processed</div>
        <div class="stat-value" id="val-totalcmds" style="color: #f43f5e;">0</div>
        <div class="stat-sub">Lifetime throughput</div>
      </div>
    </div>

    <div class="section-row">
      <!-- Live Web Console -->
      <div class="panel">
        <div class="panel-header">
          <div class="panel-title">Interactive Terminal Console</div>
          <span style="font-size: 0.75rem; color: var(--text-muted);">Execute RESP Commands</span>
        </div>
        <div class="terminal-output" id="term-out">
          &gt; Welcome to My-Redis Interactive Web Shell.<br>
          &gt; Type any Redis command (e.g. PING, SET foo bar, GET foo, INFO, GEOADD, PFADD, EVAL).<br>
        </div>
        <div class="terminal-input-bar">
          <input type="text" id="term-in" placeholder="PING" autocomplete="off" />
          <button id="term-btn" onclick="runCommand()">Execute</button>
        </div>
      </div>

      <!-- Live Keyspace Explorer -->
      <div class="panel">
        <div class="panel-header">
          <div class="panel-title">Keyspace Explorer</div>
          <span style="font-size: 0.75rem; color: var(--text-muted);" id="key-count-sub">0 keys tracked</span>
        </div>
        <div class="key-table-container">
          <table>
            <thead>
              <tr>
                <th>Key</th>
                <th>Type</th>
                <th>TTL</th>
              </tr>
            </thead>
            <tbody id="keys-tbody">
              <tr>
                <td colspan="3" style="text-align: center; color: var(--text-muted); padding: 2rem;">No keys in database</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </div>
  </main>

  <script>
    async function fetchStats() {
      try {
        const res = await fetch('/api/stats');
        const data = await res.json();
        document.getElementById('val-ops').innerText = data.ops_per_second.toFixed(1);
        document.getElementById('val-keys').innerText = data.keys_count;
        document.getElementById('val-mem').innerText = (data.memory_bytes / 1048576).toFixed(2) + ' MB';
        document.getElementById('val-hitrate').innerText = data.hit_rate.toFixed(1) + '%';
        document.getElementById('val-hitdetails').innerText = data.cache_hits + ' hits / ' + data.cache_misses + ' misses';
        document.getElementById('val-clients').innerText = data.connected_clients;
        document.getElementById('val-totalcmds').innerText = data.total_commands.toLocaleString();
        
        const hrs = Math.floor(data.uptime_seconds / 3600);
        const mins = Math.floor((data.uptime_seconds % 3600) / 60);
        const secs = data.uptime_seconds % 60;
        document.getElementById('uptime-display').innerText = 'Uptime: ' + hrs + 'h ' + mins + 'm ' + secs + 's';
      } catch (e) {
        console.error(e);
      }
    }

    async function fetchKeys() {
      try {
        const res = await fetch('/api/keys');
        const list = await res.json();
        const tbody = document.getElementById('keys-tbody');
        document.getElementById('key-count-sub').innerText = list.length + ' keys tracked';
        if (list.length === 0) {
          tbody.innerHTML = '<tr><td colspan="3" style="text-align: center; color: var(--text-muted); padding: 2rem;">No keys in database</td></tr>';
          return;
        }
        let html = '';
        for (const item of list.slice(0, 50)) {
          const typeClass = 'type-' + (item.type || 'string');
          html += '<tr><td><strong>' + escapeHtml(item.key) + '</strong></td>' +
                  '<td><span class="type-pill ' + typeClass + '">' + escapeHtml(item.type) + '</span></td>' +
                  '<td>' + (item.ttl >= 0 ? item.ttl + 's' : 'persistent') + '</td></tr>';
        }
        tbody.innerHTML = html;
      } catch (e) {
        console.error(e);
      }
    }

    async function runCommand() {
      const input = document.getElementById('term-in');
      const cmd = input.value.trim();
      if (!cmd) return;
      input.value = '';

      const term = document.getElementById('term-out');
      term.innerHTML += '<br><span style="color: #94a3b8;">&gt; ' + escapeHtml(cmd) + '</span><br>';

      try {
        const res = await fetch('/api/query', {
          method: 'POST',
          headers: {'Content-Type': 'application/json'},
          body: JSON.stringify({command: cmd})
        });
        const out = await res.json();
        term.innerHTML += '<span style="color: #38bdf8;">' + escapeHtml(out.reply) + '</span><br>';
      } catch (e) {
        term.innerHTML += '<span style="color: #f43f5e;">ERR ' + escapeHtml(e.message) + '</span><br>';
      }
      term.scrollTop = term.scrollHeight;
      fetchStats();
      fetchKeys();
    }

    document.getElementById('term-in').addEventListener('keydown', (e) => {
      if (e.key === 'Enter') runCommand();
    });

    function escapeHtml(str) {
      if (!str) return '';
      return String(str).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
    }

    setInterval(fetchStats, 1000);
    setInterval(fetchKeys, 2000);
    fetchStats();
    fetchKeys();
  </script>
</body>
</html>`
