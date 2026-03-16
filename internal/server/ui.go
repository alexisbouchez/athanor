package server

const indexHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
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

body {
  font-family: 'Ioskeley Mono', monospace;
  background: #0a0a0a;
  color: #c8c8c8;
  padding: 2rem;
  font-size: 14px;
  line-height: 1.6;
}

h1 {
  color: #e0e0e0;
  font-size: 18px;
  margin-bottom: 0.25rem;
}

.subtitle {
  color: #555;
  margin-bottom: 2rem;
  font-size: 12px;
}

.run {
  border: 1px solid #222;
  margin-bottom: 1rem;
  padding: 1rem;
}

.run-header {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
  margin-bottom: 0.5rem;
}

.run-sha { color: #888; }
.run-ref { color: #666; }
.run-actor { color: #666; font-size: 12px; }
.run-time { color: #555; font-size: 12px; }

.status {
  display: inline-block;
  padding: 0 6px;
  font-size: 12px;
  font-weight: 700;
}
.status-success { color: #4a4; }
.status-failure { color: #c44; }
.status-running { color: #4ac; }
.status-pending { color: #666; }
.status-error   { color: #c44; }
.status-skipped { color: #555; font-style: italic; }

.workflow {
  margin: 0.5rem 0 0.5rem 1rem;
}

.workflow-name {
  font-weight: 700;
  font-size: 13px;
}

.job {
  margin: 0.25rem 0 0.25rem 1rem;
}

.job-id { color: #999; }

.step {
  margin-left: 1rem;
  color: #777;
  font-size: 12px;
}

.step .glyph {
  display: inline-block;
  width: 14px;
  text-align: center;
}

.g-success { color: #4a4; }
.g-failure { color: #c44; }
.g-running { color: #4ac; }
.g-pending { color: #444; }
.g-skipped { color: #444; }

.empty {
  color: #444;
  padding: 2rem 0;
  text-align: center;
}

.blink {
  animation: blink 1s step-end infinite;
}
@keyframes blink {
  50% { opacity: 0.3; }
}
</style>
</head>
<body>

<h1>athanor</h1>
<p class="subtitle">self-hosted ci &middot; cloudhypervisor microvms</p>

<div id="runs"></div>

<script>
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

function renderRun(r) {
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
    for (const wf of r.workflows) {
      h += '<div class="workflow">';
      h += glyph(wf.status) + ' <span class="workflow-name">' + esc(wf.name) + '</span>';
      if (wf.jobs) {
        for (const j of wf.jobs) {
          h += '<div class="job">' + glyph(j.status) + ' <span class="job-id">' + esc(j.id) + '</span>';
          if (j.steps) {
            for (const s of j.steps) {
              h += '<div class="step">' + glyph(s.status) + ' ' + esc(s.name) + '</div>';
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
  el.innerHTML = runs.map(renderRun).join('');
}

// Initial load
fetch('/api/runs')
  .then(r => r.json())
  .then(render);

// Live updates via SSE
const es = new EventSource('/api/events');
es.onmessage = function(e) {
  // On any update, re-fetch the full list (simple approach)
  fetch('/api/runs')
    .then(r => r.json())
    .then(render);
};
</script>
</body>
</html>
`
