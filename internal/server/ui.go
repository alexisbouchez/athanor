package server

const indexHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="color-scheme" content="light dark">
<title>Athanor</title>
<style>
@font-face { font-family: 'Ioskeley Mono'; src: url('/font/regular.woff2') format('woff2'); font-weight: 400; }
@font-face { font-family: 'Ioskeley Mono'; src: url('/font/bold.woff2') format('woff2'); font-weight: 700; }
* { margin: 0; padding: 0; box-sizing: border-box; }

:root {
  --bg: #f5f5f4; --fg: #1c1c1c; --fg2: #444; --fg3: #777;
  --border: #d4d4d4; --run-bg: #fff; --log-bg: #fafaf9; --log-line-hover: #f0f0ee;
  --green: #1a7f37; --red: #cf222e; --blue: #0969da; --yellow: #9a6700;
  --muted: #8b8b8b; --empty: #999; --toggle-bg: #e5e5e5;
  --line-num: #c0c0c0; --error-bg: #fff0f0;
}
@media (prefers-color-scheme: dark) { :root {
  --bg: #0d0d0d; --fg: #e5e5e5; --fg2: #b0b0b0; --fg3: #888;
  --border: #2a2a2a; --run-bg: #141414; --log-bg: #111; --log-line-hover: #1a1a1a;
  --green: #3fb950; --red: #f85149; --blue: #58a6ff; --yellow: #d29922;
  --muted: #6e6e6e; --empty: #555; --toggle-bg: #222;
  --line-num: #444; --error-bg: #2d1114;
}}
html[data-theme="light"] {
  --bg: #f5f5f4; --fg: #1c1c1c; --fg2: #444; --fg3: #777;
  --border: #d4d4d4; --run-bg: #fff; --log-bg: #fafaf9; --log-line-hover: #f0f0ee;
  --green: #1a7f37; --red: #cf222e; --blue: #0969da; --yellow: #9a6700;
  --muted: #8b8b8b; --empty: #999; --toggle-bg: #e5e5e5;
  --line-num: #c0c0c0; --error-bg: #fff0f0;
}
html[data-theme="dark"] {
  --bg: #0d0d0d; --fg: #e5e5e5; --fg2: #b0b0b0; --fg3: #888;
  --border: #2a2a2a; --run-bg: #141414; --log-bg: #111; --log-line-hover: #1a1a1a;
  --green: #3fb950; --red: #f85149; --blue: #58a6ff; --yellow: #d29922;
  --muted: #6e6e6e; --empty: #555; --toggle-bg: #222;
  --line-num: #444; --error-bg: #2d1114;
}

body { font-family: 'Ioskeley Mono', monospace; background: var(--bg); color: var(--fg); font-size: 14px; line-height: 1.5; }

header { display: flex; justify-content: space-between; align-items: center; padding: 1rem 1.5rem; border-bottom: 1px solid var(--border); }
h1 { font-size: 17px; letter-spacing: 0.05em; }
nav { display: flex; gap: 0.75rem; align-items: center; }
.nav-link { color: var(--fg3); font-size: 13px; text-decoration: none; }
.nav-link:hover { color: var(--fg); }
.toggle { background: var(--toggle-bg); border: 1px solid var(--border); color: var(--fg3); font-family: inherit; font-size: 13px; padding: 2px 10px; cursor: pointer; }

#app { display: flex; height: calc(100vh - 50px); }
#sidebar { width: 360px; min-width: 300px; border-right: 1px solid var(--border); overflow-y: auto; }
#main { flex: 1; display: flex; flex-direction: column; overflow: hidden; }

.run-item { padding: 0.75rem 1rem; border-bottom: 1px solid var(--border); cursor: pointer; }
.run-item:hover { background: var(--log-bg); }
.run-item.active { background: var(--log-bg); border-left: 3px solid var(--blue); padding-left: calc(1rem - 3px); }
.run-meta { display: flex; justify-content: space-between; align-items: baseline; margin-bottom: 2px; }
.run-sha { font-weight: 700; font-size: 14px; }
.run-ref { color: var(--fg3); font-size: 13px; }
.run-info { color: var(--muted); font-size: 12px; }
.run-dur { color: var(--muted); font-size: 12px; }

.job-tree { padding: 0.5rem 1rem; overflow-y: auto; border-bottom: 1px solid var(--border); max-height: 40vh; }
.job-row { padding: 3px 0; cursor: pointer; font-size: 14px; display: flex; align-items: baseline; gap: 6px; }
.job-row:hover { color: var(--fg); }
.job-row.active { font-weight: 700; }
.step-row { padding: 2px 0 2px 1.25rem; cursor: pointer; font-size: 13px; display: flex; align-items: baseline; gap: 6px; color: var(--fg3); }
.step-row:hover { color: var(--fg2); }
.step-row.active { color: var(--fg); font-weight: 700; }
.step-dur { color: var(--muted); font-size: 12px; margin-left: auto; }

#log-panel { flex: 1; display: flex; flex-direction: column; overflow: hidden; }
.log-header { padding: 0.5rem 1rem; border-bottom: 1px solid var(--border); font-size: 14px; display: flex; justify-content: space-between; align-items: center; }
.log-title { font-weight: 700; }
.log-count { color: var(--muted); font-size: 12px; }
#log-search { background: var(--log-bg); border: 1px solid var(--border); color: var(--fg); font-family: inherit; font-size: 13px; padding: 3px 8px; width: 200px; }
#log-content { flex: 1; overflow-y: auto; font-size: 13px; line-height: 1.6; background: var(--log-bg); }
.log-line { display: flex; white-space: pre; padding: 0 1rem 0 0; }
.log-line:hover { background: var(--log-line-hover); }
.log-line.error { background: var(--error-bg); }
.log-line.search-match { background: var(--yellow); color: var(--bg); }
.line-num { display: inline-block; width: 50px; text-align: right; padding-right: 12px; color: var(--line-num); user-select: none; flex-shrink: 0; }
.line-ts { color: var(--muted); margin-right: 8px; flex-shrink: 0; }
.line-text { flex: 1; }

.g { display: inline-block; width: 12px; text-align: center; flex-shrink: 0; }
.g-success { color: var(--green); }
.g-failure { color: var(--red); }
.g-running { color: var(--blue); }
.g-pending { color: var(--muted); }
.g-skipped { color: var(--muted); }
.blink { animation: blink 1s step-end infinite; }
@keyframes blink { 50% { opacity: 0.3; } }

.empty { color: var(--empty); padding: 3rem; text-align: center; }
.live-dot { display: inline-block; width: 6px; height: 6px; border-radius: 50%; background: var(--green); margin-right: 6px; }
.live-dot.active { animation: pulse 2s ease-in-out infinite; }
@keyframes pulse { 0%,100% { opacity: 1; } 50% { opacity: 0.3; } }
</style>
</head>
<body>
<header>
  <h1>Athanor</h1>
  <nav>
    <span class="live-dot" id="liveDot"></span>
    <a class="nav-link" href="/settings">settings</a>
    <a class="nav-link" href="https://github.com/alexisbouchez/athanor">github</a>
    <button class="toggle" onclick="toggleTheme()" id="themeBtn"></button>
  </nav>
</header>
<div id="app">
  <div id="sidebar"></div>
  <div id="main">
    <div class="job-tree" id="jobTree"></div>
    <div id="log-panel">
      <div class="log-header">
        <span><span class="log-title" id="logTitle">Select a step</span> <span class="log-count" id="logCount"></span></span>
        <input type="text" id="log-search" placeholder="search logs..." oninput="filterLogs()">
      </div>
      <div id="log-content"></div>
    </div>
  </div>
</div>
<script>
function getTheme() { return localStorage.getItem('theme') || (matchMedia('(prefers-color-scheme:dark)').matches ? 'dark' : 'light'); }
function applyTheme(t) { document.documentElement.setAttribute('data-theme', t); document.getElementById('themeBtn').textContent = t === 'dark' ? 'light' : 'dark'; }
function toggleTheme() { const n = getTheme() === 'dark' ? 'light' : 'dark'; localStorage.setItem('theme', n); applyTheme(n); }
applyTheme(getTheme());

let runs = [];
let activeRun = null;
let activeJob = null;
let activeStep = null;
let searchTerm = '';
let autoScroll = true;

const G = {success:'\u2713',failure:'\u2717',running:'\u25cf',pending:'\u00b7',skipped:'\u25cb',error:'!'};
function gc(s){return 'g-'+(s||'pending');}
function blink(s){return s==='running'?' blink':'';}
function glyph(s){return '<span class="g '+gc(s)+blink(s)+'">'+(G[s]||G.pending)+'</span>';}

function ago(t){if(!t)return'';const s=Math.floor((Date.now()-new Date(t).getTime())/1000);if(s<60)return s+'s ago';if(s<3600)return Math.floor(s/60)+'m ago';if(s<86400)return Math.floor(s/3600)+'h ago';return Math.floor(s/86400)+'d ago';}
function dur(d){if(!d)return'';if(d<60)return d.toFixed(1)+'s';return Math.floor(d/60)+'m '+Math.floor(d%60)+'s';}
function ts(t){if(!t)return'';const d=new Date(t);return d.toLocaleTimeString('en',{hour12:false,hour:'2-digit',minute:'2-digit',second:'2-digit'})+'.'+String(d.getMilliseconds()).padStart(3,'0');}
function esc(s){if(!s)return'';const d=document.createElement('div');d.textContent=s;return d.innerHTML;}

function renderSidebar(){
  const el=document.getElementById('sidebar');
  if(!runs.length){el.innerHTML='<div class="empty">Waiting for builds...</div>';return;}
  el.innerHTML=runs.map((r,i)=>{
    const active=activeRun===i?' active':'';
    const d=r.duration_secs?dur(r.duration_secs):'';
    return '<div class="run-item'+active+'" onclick="selectRun('+i+')">'
      +'<div class="run-meta"><span>'+glyph(r.status)+' <span class="run-sha">'+r.sha.slice(0,8)+'</span> <span class="run-ref">'+esc(r.ref.replace('refs/heads/',''))+'</span></span>'
      +'<span class="run-dur">'+d+'</span></div>'
      +'<div class="run-info">'+esc(r.actor)+' &middot; '+r.event+' &middot; '+ago(r.started_at)+'</div>'
      +'</div>';
  }).join('');
}

function renderJobTree(){
  const el=document.getElementById('jobTree');
  if(activeRun===null||!runs[activeRun]){el.innerHTML='';return;}
  const r=runs[activeRun];
  let h='';
  for(let wi=0;wi<(r.workflows||[]).length;wi++){
    const wf=r.workflows[wi];
    h+='<div style="margin-bottom:4px">'+glyph(wf.status)+' <strong style="font-size:14px">'+esc(wf.name)+'</strong>';
    if(wf.duration_secs) h+=' <span style="color:var(--muted);font-size:12px">'+dur(wf.duration_secs)+'</span>';
    h+='</div>';
    for(let ji=0;ji<(wf.jobs||[]).length;ji++){
      const j=wf.jobs[ji];
      const jActive=activeRun!==null&&activeJob===ji&&activeStep===null?' active':'';
      h+='<div class="job-row'+jActive+'" onclick="selectJob('+wi+','+ji+')">'+glyph(j.status)+' '+esc(j.id);
      if(j.duration_secs) h+='<span class="step-dur">'+dur(j.duration_secs)+'</span>';
      h+='</div>';
      for(let si=0;si<(j.steps||[]).length;si++){
        const s=j.steps[si];
        const sActive=activeJob===ji&&activeStep===si?' active':'';
        h+='<div class="step-row'+sActive+'" onclick="selectStep('+wi+','+ji+','+si+')">'+glyph(s.status)+' '+esc(s.name);
        if(s.duration_secs) h+='<span class="step-dur">'+dur(s.duration_secs)+'</span>';
        h+='</div>';
      }
    }
  }
  el.innerHTML=h;
}

function renderLogs(){
  const el=document.getElementById('log-content');
  const title=document.getElementById('logTitle');
  const count=document.getElementById('logCount');
  if(activeRun===null||activeStep===null){
    title.textContent='Select a step to view logs';
    count.textContent='';
    el.innerHTML='';
    return;
  }
  const r=runs[activeRun];
  if(!r)return;
  let step=null;
  for(const wf of r.workflows||[]){
    for(const j of wf.jobs||[]){
      if(j===((r.workflows||[])[0]||{}).jobs?.[activeJob]){
        step=(j.steps||[])[activeStep];
      }
    }
  }
  // Find step across all workflows
  for(const wf of r.workflows||[]){
    for(const j of wf.jobs||[]){
      for(let ji=0;ji<(((r.workflows||[])).flatMap(w=>(w.jobs||[]))).length;ji++){
        // simplified: use activeJob index
      }
    }
  }
  // Direct lookup
  let foundStep=null,foundJob=null;
  outer: for(const wf of r.workflows||[]){
    for(const j of wf.jobs||[]){
      if((wf.jobs||[]).indexOf(j)===activeJob){
        foundJob=j;
        foundStep=(j.steps||[])[activeStep];
        break outer;
      }
    }
  }
  if(!foundStep){el.innerHTML='';title.textContent='';count.textContent='';return;}
  title.textContent=foundStep.name;
  const lines=foundStep.lines||[];
  count.textContent=lines.length+' lines';
  const search=searchTerm.toLowerCase();
  let h='';
  for(let i=0;i<lines.length;i++){
    const l=lines[i];
    const text=typeof l==='string'?l:(l.s||'');
    const time=typeof l==='object'?l.t:null;
    const isErr=text.startsWith('Error:')||text.includes('error')||text.includes('FAIL');
    const isMatch=search&&text.toLowerCase().includes(search);
    let cls='log-line';
    if(isErr) cls+=' error';
    if(isMatch) cls+=' search-match';
    h+='<div class="'+cls+'"><span class="line-num">'+(i+1)+'</span>';
    if(time) h+='<span class="line-ts">'+ts(time)+'</span>';
    h+='<span class="line-text">'+esc(text)+'</span></div>';
  }
  el.innerHTML=h;
  if(autoScroll&&foundStep.status==='running') el.scrollTop=el.scrollHeight;
}

function selectRun(i){activeRun=i;activeJob=null;activeStep=null;renderSidebar();renderJobTree();renderLogs();}
function selectJob(wi,ji){activeJob=ji;activeStep=null;renderJobTree();renderLogs();}
function selectStep(wi,ji,si){activeJob=ji;activeStep=si;renderJobTree();renderLogs();}
function filterLogs(){searchTerm=document.getElementById('log-search').value;renderLogs();}

// Auto-select the latest running step
function autoSelect(){
  if(!runs.length)return;
  if(activeRun===null) activeRun=0;
  const r=runs[activeRun];
  if(!r)return;
  for(const wf of r.workflows||[]){
    for(let ji=0;ji<(wf.jobs||[]).length;ji++){
      const j=wf.jobs[ji];
      for(let si=0;si<(j.steps||[]).length;si++){
        if(j.steps[si].status==='running'){activeJob=ji;activeStep=si;return;}
      }
      // If no running step, select the last completed one in a running job
      if(j.status==='running'&&j.steps&&j.steps.length){
        activeJob=ji;activeStep=j.steps.length-1;return;
      }
    }
  }
}

// Track scroll position for auto-scroll
document.addEventListener('DOMContentLoaded',()=>{
  const logEl=document.getElementById('log-content');
  logEl.addEventListener('scroll',()=>{
    autoScroll=logEl.scrollTop+logEl.clientHeight>=logEl.scrollHeight-20;
  });
});

// Initial load
fetch('/api/runs').then(r=>r.json()).then(data=>{runs=data||[];autoSelect();renderSidebar();renderJobTree();renderLogs();});

// SSE - granular event handling
// SSE needs auth token - extract from cookie
function getCookie(n){const v=document.cookie.match('(^|;)\\s*'+n+'=([^;]+)');return v?v[2]:'';}
const sseUrl='/api/events'+(getCookie('athanor_token')?'?token='+getCookie('athanor_token'):'');
const es=new EventSource(sseUrl);
const liveDot=document.getElementById('liveDot');

es.addEventListener('run',e=>{
  liveDot.classList.add('active');
  const evt=JSON.parse(e.data);
  // Full run update - refresh runs list
  fetch('/api/runs').then(r=>r.json()).then(data=>{runs=data||[];if(activeRun===null)activeRun=0;renderSidebar();renderJobTree();renderLogs();});
});

es.addEventListener('job',e=>{
  liveDot.classList.add('active');
  fetch('/api/runs').then(r=>r.json()).then(data=>{runs=data||[];renderSidebar();renderJobTree();});
});

es.addEventListener('step',e=>{
  const evt=JSON.parse(e.data);
  fetch('/api/runs').then(r=>r.json()).then(data=>{
    runs=data||[];
    // Auto-select running steps
    if(evt.data&&evt.data.status==='running') autoSelect();
    renderJobTree();renderLogs();
  });
});

es.addEventListener('log',e=>{
  const evt=JSON.parse(e.data);
  // Append log line directly for real-time streaming
  if(activeRun===0&&evt.data){
    const d=evt.data;
    // Update in-memory data
    try{
      const r=runs[0];
      if(r&&r.workflows&&r.workflows[d.wf]&&r.workflows[d.wf].jobs[d.job]){
        const step=r.workflows[d.wf].jobs[d.job].steps[d.step];
        if(step){
          if(!step.lines)step.lines=[];
          step.lines.push(d.line);
          // Append to DOM directly instead of full re-render
          const logEl=document.getElementById('log-content');
          const countEl=document.getElementById('logCount');
          if(activeJob===d.job&&activeStep===d.step){
            const idx=step.lines.length;
            const l=d.line;
            const text=l.s||'';
            const isErr=text.startsWith('Error:')||text.includes('error')||text.includes('FAIL');
            const div=document.createElement('div');
            div.className='log-line'+(isErr?' error':'');
            div.innerHTML='<span class="line-num">'+idx+'</span>'
              +(l.t?'<span class="line-ts">'+ts(l.t)+'</span>':'')
              +'<span class="line-text">'+esc(text)+'</span>';
            logEl.appendChild(div);
            countEl.textContent=idx+' lines';
            if(autoScroll) logEl.scrollTop=logEl.scrollHeight;
          }
        }
      }
    }catch(ex){}
  }
});

es.addEventListener('done',e=>{
  liveDot.classList.remove('active');
  fetch('/api/runs').then(r=>r.json()).then(data=>{runs=data||[];renderSidebar();renderJobTree();renderLogs();});
});

// Fallback: re-render periodically for timestamps
setInterval(()=>{renderSidebar();},30000);
</script>
</body>
</html>
`
