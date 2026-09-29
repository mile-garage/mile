// Copyright (C) 2026 Gabriele Menghi
// SPDX-License-Identifier: AGPL-3.0-only

// Service worker: keeps the interface (HTML, JS, CSS, icons) so that the app
// opens without a connection. The API, attachments and calendar feed never go
// through the cache: data is always fresh and nothing stays after logging out.
//
// - pages: network first, the cached page when offline;
// - /assets/ (hashed names, never change): cache first;
// - icons and manifest: cache, refreshed in the background.
// This file does not change between releases: a new version of MILE is picked
// up with the page, and the files it no longer uses are removed.

const CACHE = 'mile-ui-v1'
const STATIC = ['/manifest.webmanifest', '/favicon.png', '/apple-touch-icon.png', '/icon-192.png', '/icon-512.png', '/logo-m.png']

// assetsOf lists the /assets/ files an index.html loads.
function assetsOf(html) {
  return [...new Set(html.match(/\/assets\/[^"'\s)]+/g) || [])]
}

// savePage caches index.html with its assets, and drops the assets of older versions.
async function savePage(res) {
  const cache = await caches.open(CACHE)
  const html = await res.clone().text()
  const assets = assetsOf(html)
  const missing = []
  for (const a of assets) if (!(await cache.match(a))) missing.push(a)
  await cache.addAll(missing)
  await cache.put('/', res)
  for (const req of await cache.keys()) {
    const path = new URL(req.url).pathname
    if (path.startsWith('/assets/') && !assets.includes(path)) await cache.delete(req)
  }
}

self.addEventListener('install', (event) => {
  event.waitUntil(
    (async () => {
      const cache = await caches.open(CACHE)
      await cache.addAll(STATIC)
      const res = await fetch('/', { cache: 'no-cache' })
      if (res.ok) await savePage(res)
      await self.skipWaiting()
    })(),
  )
})

self.addEventListener('activate', (event) => {
  event.waitUntil(
    (async () => {
      for (const name of await caches.keys()) if (name !== CACHE) await caches.delete(name)
      await self.clients.claim()
    })(),
  )
})

self.addEventListener('fetch', (event) => {
  const req = event.request
  const url = new URL(req.url)
  if (req.method !== 'GET' || url.origin !== location.origin) return
  const path = url.pathname
  if (path.startsWith('/api/') || path.startsWith('/auth/') || path.startsWith('/calendar/') || path === '/healthz' || path === '/sw.js') return

  if (req.mode === 'navigate') {
    event.respondWith(
      (async () => {
        try {
          const res = await fetch(req)
          // Every route of the app is the same index.html.
          if (res.ok && (res.headers.get('Content-Type') || '').startsWith('text/html')) event.waitUntil(savePage(res.clone()))
          return res
        } catch (e) {
          const cached = await caches.match('/')
          if (cached) return cached
          throw e
        }
      })(),
    )
    return
  }

  if (path.startsWith('/assets/')) {
    event.respondWith(
      (async () => {
        const cached = await caches.match(req)
        if (cached) return cached
        const res = await fetch(req)
        if (res.ok) {
          const copy = res.clone()
          event.waitUntil(caches.open(CACHE).then((c) => c.put(req, copy)))
        }
        return res
      })(),
    )
    return
  }

  if (STATIC.includes(path)) {
    event.respondWith(
      (async () => {
        const cached = await caches.match(req)
        const fresh = fetch(req).then(async (res) => {
          if (res.ok) await (await caches.open(CACHE)).put(req, res.clone())
          return res
        })
        if (cached) {
          event.waitUntil(fresh.catch(() => {}))
          return cached
        }
        return fresh
      })(),
    )
  }
})
