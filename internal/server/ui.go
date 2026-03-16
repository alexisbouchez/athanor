package server

const indexHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="color-scheme" content="light dark">
<title>athanor</title>
<style>
@font-face {
  font-family: 'Ioskeley Mono';
  src: url('/font/regular.woff2') format('woff2');
  font-weight: 400;
}
@font-face {
  font-family: 'Ioskeley Mono';
  src: url('/font/bold.woff2') format('woff2');
  font-weight: 700;
}

* { margin: 0; padding: 0; box-sizing: border-box; }

:root {
  --bg: #f5f5f4;
  --fg: #1c1c1c;
  --fg2: #444;
  --fg3: #777;
  --border: #d4d4d4;
  --run-bg: #fff;
  --log-bg: #f0f0ee;
  --green: #1a7f37;
  --red: #cf222e;
  --blue: #0969da;
  --muted: #8b8b8b;
  --empty: #999;
  --toggle-bg: #e5e5e5;
}

@media (prefers-color-scheme: dark) {
  :root {
    --bg: #0d0d0d;
    --fg: #e5e5e5;
    --fg2: #b0b0b0;
    --fg3: #888;
    --border: #2a2a2a;
    --run-bg: #141414;
    --log-bg: #1a1a1a;
    --green: #3fb950;
    --red: #f85149;
    --blue: #58a6ff;
    --muted: #6e6e6e;
    --empty: #555;
    --toggle-bg: #222;
  }
}

html[data-theme="light"] {
  --bg: #f5f5f4;
  --fg: #1c1c1c;
  --fg2: #444;
  --fg3: #777;
  --border: #d4d4d4;
  --run-bg: #fff;
  --log-bg: #f0f0ee;
  --green: #1a7f37;
  --red: #cf222e;
  --blue: #0969da;
  --muted: #8b8b8b;
  --empty: #999;
  --toggle-bg: #e5e5e5;
}

html[data-theme="dark"] {
  --bg: #0d0d0d;
  --fg: #e5e5e5;
  --fg2: #b0b0b0;
  --fg3: #888;
  --border: #2a2a2a;
  --run-bg: #141414;
  --log-bg: #1a1a1a;
  --green: #3fb950;
  --red: #f85149;
  --blue: #58a6ff;
  --muted: #6e6e6e;
  --empty: #555;
  --toggle-bg: #222;
}

body {
  font-family: 'Ioskeley Mono', monospace;
  background: var(--bg);
  color: var(--fg);
  padding: 2rem;
  font-size: 14px;
  line-height: 1.6;
}

header {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
  margin-bottom: 0.25rem;
}

h1 { font-size: 18px; }

.toggle {
  background: var(--toggle-bg);
  border: 1px solid var(--border);
  color: var(--fg3);
  font-family: inherit;
  font-size: 12px;
  padding: 2px 10px;
  cursor: pointer;
}

.subtitle {
  color: var(--fg3);
  margin-bottom: 2rem;
  font-size: 12px;
}

.run {
  border: 1px solid var(--border);
  background: var(--run-bg);
  margin-bottom: 1rem;
  padding: 1rem;
}

.run-header {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
  margin-bottom: 0.5rem;
}

.run-sha { color: var(--fg2); font-weight: 700; }
.run-ref { color: var(--fg3); }
.run-actor { color: var(--fg3); font-size: 12px; }
.run-time { color: var(--muted); font-size: 12px; }

.status {
  display: inline-block;
  padding: 0 6px;
  font-size: 12px;
  font-weight: 700;
}
.status-success { color: var(--green); }
.status-failure { color: var(--red); }
.status-running { color: var(--blue); }
.status-pending { color: var(--muted); }
.status-error   { color: var(--red); }
.status-skipped { color: var(--muted); font-style: italic; }

.workflow { margin: 0.5rem 0 0.5rem 1rem; }
.workflow-name { font-weight: 700; font-size: 13px; }
.job { margin: 0.25rem 0 0.25rem 1rem; }
.job-id { color: var(--fg2); }

.step {
  margin-left: 1rem;
  font-size: 12px;
}
.step-header {
  color: var(--fg3);
  cursor: pointer;
  user-select: none;
}
.step-header:hover { color: var(--fg2); }
.step-header .glyph { display: inline-block; width: 14px; text-align: center; }
.step-header .has-log { border-bottom: 1px dotted var(--muted); }

.step-log {
  display: none;
  background: var(--log-bg);
  border: 1px solid var(--border);
  padding: 0.5rem;
  margin: 0.25rem 0 0.5rem 1rem;
  font-size: 11px;
  line-height: 1.5;
  overflow-x: auto;
  white-space: pre;
  color: var(--fg2);
  max-height: 400px;
  overflow-y: auto;
}
.step-log.open { display: block; }

.log-line { display: block; }
.log-line:hover { background: var(--border); }

.g-success { color: var(--green); }
.g-failure { color: var(--red); }
.g-running { color: var(--blue); }
.g-pending { color: var(--muted); }
.g-skipped { color: var(--muted); }

.empty { color: var(--empty); padding: 2rem 0; text-align: center; }

.blink { animation: blink 1s step-end infinite; }
@keyframes blink { 50% { opacity: 0.3; } }
</style>
</head>
<body>

<header>
  <h1>athanor</h1>
  <button class="toggle" onclick="toggleTheme()" id="themeBtn"></button>
</header>
<p class="subtitle">self-hosted ci &middot; cloudhypervisor microvms</p>

<div id="runs"></div>

<script>
function getTheme() {
  const saved = localStorage.getItem('theme');
  if (saved) return saved;
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
}

function applyTheme(t) {
  document.documentElement.setAttribute('data-theme', t);
  document.getElementById('themeBtn').textContent = t === 'dark' ? 'light' : 'dark';
}

function toggleTheme() {
  const next = getTheme() === 'dark' ? 'light' : 'dark';
  localStorage.setItem('theme', next);
  applyTheme(next);
}

applyTheme(getTheme());

const G = {
  success: '<span class="glyph g-success">&#10003;</span>',
  failure: '<span class="glyph g-failure">&#10007;</span>',
  running: '<span class="glyph g-running blink">&#9679;</span>',
  pending: '<span class="glyph g-pending">&#183;</span>',
  skipped: '<span class="glyph g-skipped">&#9675;</span>',
  error:   '<span class="glyph g-failure">!</span>',
};

function glyph(s) { return G[s] || G.pending; }

function ago(t) {
  const s = Math.floor((Date.now() - new Date(t).getTime()) / 1000);
  if (s < 60) return s + 's ago';
  if (s < 3600) return Math.floor(s/60) + 'm ago';
  if (s < 86400) return Math.floor(s/3600) + 'h ago';
  return Math.floor(s/86400) + 'd ago';
}

function dur(d) {
  if (!d) return '';
  if (d < 60) return d.toFixed(1) + 's';
  return Math.floor(d/60) + 'm ' + Math.floor(d%60) + 's';
}

let openLogs = new Set();

function toggleLog(id) {
  const el = document.getElementById(id);
  if (!el) return;
  if (el.classList.contains('open')) {
    el.classList.remove('open');
    openLogs.delete(id);
  } else {
    el.classList.add('open');
    openLogs.add(id);
  }
}

function renderRun(r, ri) {
  let h = '<div class="run">';
  h += '<div class="run-header">';
  h += '<span>' + glyph(r.status) + ' <span class="run-sha">' + r.sha.slice(0,8) + '</span>';
  h += ' <span class="run-ref">' + esc(r.ref.replace('refs/heads/','')) + '</span></span>';
  h += '<span class="run-time">' + ago(r.started_at);
  if (r.duration_secs) h += ' &middot; ' + dur(r.duration_secs);
  h += '</span>';
  h += '</div>';
  h += '<div class="run-actor">' + esc(r.actor) + ' &middot; ' + r.event + ' &middot; <span class="status status-' + r.status + '">' + r.status + '</span></div>';

  if (r.workflows) {
    for (let wi = 0; wi < r.workflows.length; wi++) {
      const wf = r.workflows[wi];
      h += '<div class="workflow">';
      h += glyph(wf.status) + ' <span class="workflow-name">' + esc(wf.name) + '</span>';
      if (wf.jobs) {
        for (let ji = 0; ji < wf.jobs.length; ji++) {
          const j = wf.jobs[ji];
          h += '<div class="job">' + glyph(j.status) + ' <span class="job-id">' + esc(j.id) + '</span>';
          if (j.steps) {
            for (let si = 0; si < j.steps.length; si++) {
              const s = j.steps[si];
              const logId = 'log-' + ri + '-' + wi + '-' + ji + '-' + si;
              const hasLog = s.lines && s.lines.length > 0;
              const isOpen = openLogs.has(logId);
              h += '<div class="step">';
              h += '<div class="step-header" onclick="toggleLog(\'' + logId + '\')">';
              h += glyph(s.status) + ' <span' + (hasLog ? ' class="has-log"' : '') + '>' + esc(s.name) + '</span>';
              if (hasLog) h += ' <span style="color:var(--muted);font-size:10px">(' + s.lines.length + ' lines)</span>';
              h += '</div>';
              if (hasLog) {
                h += '<div class="step-log' + (isOpen ? ' open' : '') + '" id="' + logId + '">';
                for (const line of s.lines) {
                  h += '<span class="log-line">' + esc(line) + '</span>';
                }
                h += '</div>';
              }
              h += '</div>';
            }
          }
          h += '</div>';
        }
      }
      h += '</div>';
    }
  }
  h += '</div>';
  return h;
}

function esc(s) {
  if (!s) return '';
  const d = document.createElement('div');
  d.textContent = s;
  return d.innerHTML;
}

function render(runs) {
  const el = document.getElementById('runs');
  if (!runs || runs.length === 0) {
    el.innerHTML = '<div class="empty">no runs yet &mdash; push to trigger a build</div>';
    return;
  }
  el.innerHTML = runs.map((r, i) => renderRun(r, i)).join('');
}

fetch('/api/runs').then(r => r.json()).then(render);

const es = new EventSource('/api/events');
es.onmessage = function(e) {
  fetch('/api/runs').then(r => r.json()).then(render);
};
</script>
</body>
</html>
`
