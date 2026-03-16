package server

const loginHTML = `<!DOCTYPE html>
<html><head>
<meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Athanor</title>
<style>
@font-face { font-family: 'Ioskeley Mono'; src: url('/font/regular.woff2') format('woff2'); font-weight: 400; }
* { margin: 0; padding: 0; box-sizing: border-box; }
body { font-family: 'Ioskeley Mono', monospace; background: #0d0d0d; color: #e5e5e5; display: flex; align-items: center; justify-content: center; height: 100vh; }
form { text-align: center; }
h1 { font-size: 16px; margin-bottom: 1.5rem; }
input[type=password] { background: #141414; border: 1px solid #2a2a2a; color: #e5e5e5; font-family: inherit; font-size: 13px; padding: 8px 12px; width: 280px; display: block; margin: 0 auto 1rem; }
button { background: #222; border: 1px solid #2a2a2a; color: #e5e5e5; font-family: inherit; font-size: 13px; padding: 8px 20px; cursor: pointer; }
button:hover { background: #333; }
</style>
</head><body>
<form method="POST" action="/login">
<h1>Athanor</h1>
<input type="password" name="token" placeholder="dashboard token" autofocus>
<button type="submit">enter</button>
</form>
</body></html>
`
