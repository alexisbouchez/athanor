package server

const settingsHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="color-scheme" content="light dark">
<title>Athanor - Settings</title>
<style>
@font-face { font-family: 'Ioskeley Mono'; src: url('/font/regular.woff2') format('woff2'); font-weight: 400; }
@font-face { font-family: 'Ioskeley Mono'; src: url('/font/bold.woff2') format('woff2'); font-weight: 700; }
* { margin: 0; padding: 0; box-sizing: border-box; }

:root {
  --bg: #f5f5f4; --fg: #1c1c1c; --fg2: #444; --fg3: #777;
  --border: #d4d4d4; --card-bg: #fff; --input-bg: #fafaf9;
  --green: #1a7f37; --red: #cf222e; --blue: #0969da;
  --muted: #8b8b8b; --toggle-bg: #e5e5e5;
}
@media (prefers-color-scheme: dark) { :root {
  --bg: #0d0d0d; --fg: #e5e5e5; --fg2: #b0b0b0; --fg3: #888;
  --border: #2a2a2a; --card-bg: #141414; --input-bg: #1a1a1a;
  --green: #3fb950; --red: #f85149; --blue: #58a6ff;
  --muted: #6e6e6e; --toggle-bg: #222;
}}
html[data-theme="light"] {
  --bg: #f5f5f4; --fg: #1c1c1c; --fg2: #444; --fg3: #777;
  --border: #d4d4d4; --card-bg: #fff; --input-bg: #fafaf9;
  --green: #1a7f37; --red: #cf222e; --blue: #0969da;
  --muted: #8b8b8b; --toggle-bg: #e5e5e5;
}
html[data-theme="dark"] {
  --bg: #0d0d0d; --fg: #e5e5e5; --fg2: #b0b0b0; --fg3: #888;
  --border: #2a2a2a; --card-bg: #141414; --input-bg: #1a1a1a;
  --green: #3fb950; --red: #f85149; --blue: #58a6ff;
  --muted: #6e6e6e; --toggle-bg: #222;
}

body { font-family: 'Ioskeley Mono', monospace; background: var(--bg); color: var(--fg); font-size: 14px; line-height: 1.6; }
header { display: flex; justify-content: space-between; align-items: center; padding: 1rem 1.5rem; border-bottom: 1px solid var(--border); }
h1 { font-size: 17px; }
h2 { font-size: 15px; margin-bottom: 1rem; }
nav { display: flex; gap: 0.75rem; align-items: center; }
.nav-link { color: var(--fg3); font-size: 13px; text-decoration: none; }
.nav-link:hover { color: var(--fg); }
.toggle { background: var(--toggle-bg); border: 1px solid var(--border); color: var(--fg3); font-family: inherit; font-size: 13px; padding: 2px 10px; cursor: pointer; }

main { max-width: 720px; margin: 2rem auto; padding: 0 1.5rem; }
.card { background: var(--card-bg); border: 1px solid var(--border); padding: 1.25rem; margin-bottom: 1.5rem; }

label { display: block; font-size: 13px; color: var(--fg2); margin-bottom: 4px; }
input[type=text], input[type=password] {
  background: var(--input-bg); border: 1px solid var(--border); color: var(--fg);
  font-family: inherit; font-size: 13px; padding: 6px 10px; width: 100%;
  margin-bottom: 0.75rem;
}
.row { display: flex; gap: 0.5rem; align-items: flex-end; margin-bottom: 0.5rem; }
.row input { flex: 1; margin-bottom: 0; }
.btn {
  background: var(--toggle-bg); border: 1px solid var(--border); color: var(--fg);
  font-family: inherit; font-size: 13px; padding: 6px 14px; cursor: pointer;
  white-space: nowrap;
}
.btn:hover { background: var(--border); }
.btn-primary { background: var(--fg); color: var(--bg); border-color: var(--fg); }
.btn-primary:hover { opacity: 0.9; }
.btn-danger { color: var(--red); }
.btn-danger:hover { background: var(--red); color: var(--bg); border-color: var(--red); }

.secret-row { display: flex; gap: 0.5rem; align-items: center; padding: 6px 0; border-bottom: 1px solid var(--border); font-size: 13px; }
.secret-key { font-weight: 700; flex: 1; }
.secret-mask { color: var(--muted); }
.msg { font-size: 12px; padding: 6px 0; }
.msg-ok { color: var(--green); }
.msg-err { color: var(--red); }
.empty { color: var(--muted); font-size: 13px; padding: 1rem 0; }
</style>
</head>
<body>
<header>
  <h1>Athanor</h1>
  <nav>
    <a class="nav-link" href="/">builds</a>
    <a class="nav-link" href="https://github.com/alexisbouchez/athanor">github</a>
    <button class="toggle" onclick="toggleTheme()" id="themeBtn"></button>
  </nav>
</header>

<main>
  <h2>Secrets</h2>
  <p style="color:var(--fg3);font-size:13px;margin-bottom:1.5rem">
    Per-repo secrets are available as <code>secrets.*</code> in workflows.
    Values are stored encrypted on disk and never displayed after saving.
  </p>

  <div class="card">
    <label>Repository</label>
    <div class="row">
      <input type="text" id="repo" placeholder="owner/repo">
      <button class="btn" onclick="loadSecrets()">load</button>
    </div>
  </div>

  <div class="card" id="secretsCard" style="display:none">
    <h2 id="secretsTitle"></h2>
    <div id="secretsList"></div>
    <div style="margin-top:1rem;border-top:1px solid var(--border);padding-top:1rem">
      <label>Add secret</label>
      <div class="row">
        <input type="text" id="newKey" placeholder="KEY">
        <input type="password" id="newValue" placeholder="value">
        <button class="btn btn-primary" onclick="addSecret()">save</button>
      </div>
    </div>
    <div class="msg" id="msg"></div>
  </div>
</main>

<script>
function getTheme(){return localStorage.getItem('theme')||(matchMedia('(prefers-color-scheme:dark)').matches?'dark':'light');}
function applyTheme(t){document.documentElement.setAttribute('data-theme',t);document.getElementById('themeBtn').textContent=t==='dark'?'light':'dark';}
function toggleTheme(){const n=getTheme()==='dark'?'light':'dark';localStorage.setItem('theme',n);applyTheme(n);}
applyTheme(getTheme());

let currentRepo = '';
let currentSecrets = {};

function msg(text, ok) {
  const el = document.getElementById('msg');
  el.textContent = text;
  el.className = 'msg ' + (ok ? 'msg-ok' : 'msg-err');
  setTimeout(() => el.textContent = '', 3000);
}

async function loadSecrets() {
  const repo = document.getElementById('repo').value.trim();
  if (!repo || !repo.includes('/')) { msg('enter owner/repo', false); return; }
  currentRepo = repo;
  document.getElementById('secretsCard').style.display = 'block';
  document.getElementById('secretsTitle').textContent = repo;

  const res = await fetch('/api/secrets/' + encodeURIComponent(repo));
  const keys = await res.json();
  currentSecrets = {};
  (keys || []).forEach(k => currentSecrets[k] = true);
  renderSecrets();
}

function renderSecrets() {
  const el = document.getElementById('secretsList');
  const keys = Object.keys(currentSecrets);
  if (!keys.length) {
    el.innerHTML = '<div class="empty">No secrets configured for this repository.</div>';
    return;
  }
  el.innerHTML = keys.map(k =>
    '<div class="secret-row">' +
    '<span class="secret-key">' + esc(k) + '</span>' +
    '<span class="secret-mask">********</span>' +
    '<button class="btn btn-danger" onclick="deleteSecret(\'' + esc(k) + '\')">delete</button>' +
    '</div>'
  ).join('');
}

async function addSecret() {
  const key = document.getElementById('newKey').value.trim();
  const value = document.getElementById('newValue').value;
  if (!key) { msg('key required', false); return; }
  if (!value) { msg('value required', false); return; }

  // Fetch current secrets, add new one, save all
  const existing = await (await fetch('/api/secrets/' + encodeURIComponent(currentRepo))).json();
  const secrets = {};
  // We need to re-send all secrets. Since we don't have values for existing ones,
  // we can only add new ones. For a full replace, we'd need to store values client-side.
  // Instead, use the Set endpoint approach: send just the new key.
  // Actually, let's use PUT with all secrets. For existing keys we don't have values,
  // so we'll do a merge server-side.

  // Simpler: just POST the single key-value pair
  const body = {};
  // Get existing from server (which has values)
  const existingFull = await fetch('/api/secrets/' + encodeURIComponent(currentRepo) + '?action=get_full', {method: 'GET'});

  // Let's just PUT the single new secret merged
  body[key] = value;
  const res = await fetch('/api/secrets/' + encodeURIComponent(currentRepo), {
    method: 'PUT',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify(body),
  });

  if (res.ok) {
    msg('saved ' + key, true);
    document.getElementById('newKey').value = '';
    document.getElementById('newValue').value = '';
    currentSecrets[key] = true;
    renderSecrets();
  } else {
    msg('failed to save', false);
  }
}

async function deleteSecret(key) {
  const res = await fetch('/api/secrets/' + encodeURIComponent(currentRepo) + '?key=' + encodeURIComponent(key), {method: 'DELETE'});
  if (res.ok) {
    msg('deleted ' + key, true);
    delete currentSecrets[key];
    renderSecrets();
  } else {
    msg('failed to delete', false);
  }
}

function esc(s) { const d = document.createElement('div'); d.textContent = s; return d.innerHTML; }
</script>
</body>
</html>
`
