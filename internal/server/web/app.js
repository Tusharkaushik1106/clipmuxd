const $ = (id) => document.getElementById(id);
const toastEl = $('toast');
const dot = $('dot');

const TABS = [
  { id: 'all',     label: 'All',     match: () => true },
  { id: 'text',    label: 'Text',    match: (i) => i.category === 'text' },
  { id: 'link',    label: 'Links',   match: (i) => i.category === 'link' },
  { id: 'image',   label: 'Images',  match: (i) => i.category === 'image' },
  { id: 'video',   label: 'Videos',  match: (i) => i.category === 'video' },
  { id: 'audio',   label: 'Audio',   match: (i) => i.category === 'audio' },
  { id: 'doc',     label: 'Docs',    match: (i) => i.category === 'doc' },
  { id: 'archive', label: 'Archives',match: (i) => i.category === 'archive' },
  { id: 'other',   label: 'Other',   match: (i) => i.category === 'other' },
];

let activeTab = localStorage.getItem('ec_tab') || 'all';
let cachedItems = [];

function toast(msg, ok=true) {
  toastEl.textContent = msg;
  toastEl.style.background = ok ? '#22c55e' : '#ef4444';
  toastEl.style.color = ok ? '#052e13' : '#fff';
  toastEl.classList.add('show');
  setTimeout(() => toastEl.classList.remove('show'), 1800);
}

async function sendText() {
  const t = $('text').value.trim();
  if (!t) return;
  const res = await fetch('/api/send', { method: 'POST', body: t, headers: {'Content-Type': 'text/plain'} });
  if (res.ok) { $('text').value = ''; toast('Sent → copied to PC clipboard'); loadInbox(); }
  else toast('Failed: ' + res.status, false);
}

async function sendFiles() {
  const files = $('file').files;
  if (!files || !files.length) return;
  for (const f of files) await uploadOne(f);
  $('file').value = '';
  loadInbox();
}

function uploadOne(file) {
  return new Promise((resolve) => {
    const fd = new FormData();
    fd.append('file', file, file.name);
    const xhr = new XMLHttpRequest();
    xhr.open('POST', '/api/send');
    const prog = $('prog'); const bar = prog.firstElementChild;
    prog.classList.add('on'); bar.style.width = '0%';
    xhr.upload.onprogress = (e) => { if (e.lengthComputable) bar.style.width = (e.loaded / e.total * 100) + '%'; };
    xhr.onload = () => {
      prog.classList.remove('on');
      if (xhr.status >= 200 && xhr.status < 300) toast('Uploaded: ' + file.name);
      else toast('Failed: ' + file.name, false);
      resolve();
    };
    xhr.onerror = () => { prog.classList.remove('on'); toast('Network error', false); resolve(); };
    xhr.send(fd);
  });
}

function fmtSize(n) {
  if (n < 1024) return n + ' B';
  if (n < 1024*1024) return (n/1024).toFixed(1) + ' KB';
  if (n < 1024*1024*1024) return (n/1024/1024).toFixed(1) + ' MB';
  return (n/1024/1024/1024).toFixed(2) + ' GB';
}
function escapeHtml(s) {
  return s.replace(/[&<>"']/g, (c) => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
}

async function loadInbox() {
  try {
    const res = await fetch('/api/inbox');
    if (!res.ok) return;
    cachedItems = await res.json();
    render();
  } catch {}
}

function render() {
  renderTabs();
  const box = $('inbox');
  const tab = TABS.find(t => t.id === activeTab) || TABS[0];
  const items = cachedItems.filter(tab.match);
  if (!items.length) { box.innerHTML = '<div class="empty">nothing here yet</div>'; return; }

  if (activeTab === 'image' || activeTab === 'video') {
    box.innerHTML = '';
    const grid = document.createElement('div');
    grid.className = 'grid';
    for (const it of items) grid.appendChild(thumbEl(it));
    box.appendChild(grid);
    return;
  }

  box.innerHTML = '';
  for (const it of items) box.appendChild(rowEl(it));
}

function renderTabs() {
  const counts = {};
  for (const t of TABS) counts[t.id] = cachedItems.filter(t.match).length;
  $('tabs').innerHTML = TABS.map(t =>
    `<button data-tab="${t.id}" class="${t.id === activeTab ? 'active' : ''}">${t.label}<span class="count">${counts[t.id]}</span></button>`
  ).join('');
}

function thumbEl(it) {
  const el = document.createElement('div');
  el.className = 'thumb';
  const src = '/api/raw/' + encodeURIComponent(it.id);
  if (it.category === 'image') {
    el.innerHTML = `<img src="${src}" loading="lazy">`;
    el.addEventListener('click', (e) => { if (!e.target.closest('button')) openModal(`<img src="${src}">`); });
  } else {
    el.innerHTML = `<video src="${src}" preload="metadata"></video>`;
    el.addEventListener('click', (e) => { if (!e.target.closest('button')) openModal(`<video src="${src}" controls autoplay></video>`); });
  }
  const bar = document.createElement('div');
  bar.className = 'bar';
  bar.innerHTML = `
    <button data-act="dl" data-id="${encodeURIComponent(it.id)}" title="Download">⬇</button>
    <button class="danger" data-act="del" data-id="${encodeURIComponent(it.id)}" title="Delete">✕</button>`;
  el.appendChild(bar);
  const name = document.createElement('div');
  name.className = 'name';
  name.textContent = it.name;
  el.appendChild(name);
  return el;
}

function rowEl(it) {
  const el = document.createElement('div');
  el.className = 'item';
  const d = new Date(it.modified * 1000);
  let body = '';
  if (it.category === 'link' && it.url) {
    body = `<div class="link-row"><a href="${escapeHtml(it.url)}" target="_blank" rel="noopener">${escapeHtml(it.url)}</a></div>`;
  } else if (it.preview) {
    body = `<pre>${escapeHtml(it.preview)}</pre>`;
  }
  el.innerHTML = `
    <div style="flex:1; min-width:0;">
      <div><strong>${escapeHtml(it.name)}</strong></div>
      <div class="meta">${it.category} · ${fmtSize(it.size)} · ${d.toLocaleString()}</div>
      ${body}
    </div>
    <div class="actions">
      ${it.kind === 'text' ? `<button data-act="copy" data-id="${encodeURIComponent(it.id)}">Copy</button>` : ''}
      ${it.category === 'link' && it.url ? `<button data-act="open" data-url="${escapeHtml(it.url)}">Open</button>` : ''}
      <button data-act="dl" data-id="${encodeURIComponent(it.id)}">Download</button>
      <button class="danger" data-act="del" data-id="${encodeURIComponent(it.id)}">Del</button>
    </div>`;
  return el;
}

function openModal(html) {
  $('modalBody').innerHTML = html;
  $('modal').classList.add('on');
}
function closeModal() {
  $('modal').classList.remove('on');
  $('modalBody').innerHTML = '';
}
$('modalClose').addEventListener('click', closeModal);
$('modal').addEventListener('click', (e) => { if (e.target === $('modal')) closeModal(); });

document.addEventListener('click', async (e) => {
  const tabBtn = e.target.closest('#tabs button[data-tab]');
  if (tabBtn) {
    activeTab = tabBtn.dataset.tab;
    localStorage.setItem('ec_tab', activeTab);
    render();
    return;
  }
  const b = e.target.closest('button[data-act]');
  if (!b) return;
  const id = b.dataset.id;
  if (b.dataset.act === 'dl') {
    window.location.href = '/api/inbox/' + id;
  } else if (b.dataset.act === 'del') {
    await fetch('/api/delete/' + id, { method: 'POST' });
    loadInbox();
  } else if (b.dataset.act === 'copy') {
    const res = await fetch('/api/inbox/' + id);
    const txt = await res.text();
    try { await navigator.clipboard.writeText(txt); toast('Copied'); }
    catch { toast('Copy blocked', false); }
  } else if (b.dataset.act === 'open') {
    window.open(b.dataset.url, '_blank', 'noopener');
  }
});

$('sendText').addEventListener('click', sendText);
$('sendFile').addEventListener('click', sendFiles);
$('refresh').addEventListener('click', loadInbox);
$('pasteBtn').addEventListener('click', async () => {
  try { const t = await navigator.clipboard.readText(); if (t) $('text').value = t; }
  catch { toast('Clipboard read blocked', false); }
});
$('text').addEventListener('keydown', (e) => {
  if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') sendText();
});

let es;
function connectSSE() {
  try { es && es.close(); } catch {}
  es = new EventSource('/api/ws');
  es.addEventListener('hello', () => dot.classList.add('on'));
  es.addEventListener('update', () => loadInbox());
  es.onerror = () => { dot.classList.remove('on'); setTimeout(connectSSE, 2000); };
}
connectSSE();
loadInbox();

if ('serviceWorker' in navigator) {
  navigator.serviceWorker.register('/sw.js').catch(()=>{});
}
