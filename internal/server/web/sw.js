// Minimal SW: shell cache + share target handler.
const CACHE = 'ec-v1';
const SHELL = ['/', '/app.js', '/manifest.webmanifest'];

self.addEventListener('install', (e) => {
  e.waitUntil(caches.open(CACHE).then(c => c.addAll(SHELL)).catch(()=>{}));
  self.skipWaiting();
});
self.addEventListener('activate', (e) => { self.clients.claim(); });

self.addEventListener('fetch', (e) => {
  const url = new URL(e.request.url);

  // Share target: POST /share
  if (url.pathname === '/share' && e.request.method === 'POST') {
    e.respondWith(handleShare(e.request));
    return;
  }

  if (e.request.method !== 'GET') return;
  if (url.pathname.startsWith('/api/')) return;
  e.respondWith(
    caches.match(e.request).then(r => r || fetch(e.request).catch(() => caches.match('/')))
  );
});

async function handleShare(req) {
  const form = await req.formData();
  const text = [form.get('title'), form.get('text'), form.get('url')].filter(Boolean).join('\n');
  const files = form.getAll('file');

  if (text) {
    await fetch('/api/send', { method: 'POST', body: text, headers: {'Content-Type':'text/plain'} });
  }
  for (const f of files) {
    if (f && f.size > 0) {
      const fd = new FormData();
      fd.append('file', f, f.name || 'shared');
      await fetch('/api/send', { method: 'POST', body: fd });
    }
  }
  return Response.redirect('/', 303);
}
