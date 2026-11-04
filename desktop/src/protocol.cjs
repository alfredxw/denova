const ORIGIN = 'https://denova.localhost'

function isAppURL(value) {
  try {
    const url = new URL(value)
    return url.origin === ORIGIN && !url.username && !url.password
  } catch { return false }
}

// Keep Chromium storage on one stable origin while the OS assigns a free Go
// port. Stream responses directly so SSE and large media are never buffered.
async function forwardRequest(request, backend, { rendererOrigin } = {}) {
  const origin = request.headers.get('origin')
  if (!isAppURL(request.url) || (origin && origin !== ORIGIN)) return new Response(null, { status: 403 })
  const source = new URL(request.url)
  const useVite = rendererOrigin && source.pathname !== '/api' && !source.pathname.startsWith('/api/')
  if (!useVite && (!backend?.url || backend.closed || backend.stopping)) return new Response(null, { status: 503 })
  const target = new URL(useVite ? rendererOrigin : backend.url)
  target.pathname = source.pathname
  target.search = source.search
  const headers = new Headers(request.headers)
  for (const header of ['host', 'cookie', 'referer', 'connection', 'content-length']) headers.delete(header)
  if (useVite) {
    headers.delete('authorization')
    headers.delete('x-denova-desktop')
  } else {
    headers.set('Origin', ORIGIN)
    headers.set('X-Denova-Desktop', backend.token)
  }
  const body = request.method === 'GET' || request.method === 'HEAD' ? undefined : request.body
  const response = await fetch(target, { method: request.method, headers, body, duplex: 'half', signal: request.signal, redirect: 'manual' })
  const responseHeaders = new Headers(response.headers)
  for (const header of ['set-cookie', 'content-encoding', 'content-length', 'transfer-encoding', 'connection']) responseHeaders.delete(header)
  if (useVite) responseHeaders.set('Cross-Origin-Resource-Policy', 'same-origin')
  return new Response(response.body, { status: response.status, statusText: response.statusText, headers: responseHeaders })
}

module.exports = { ORIGIN, isAppURL, forwardRequest }
