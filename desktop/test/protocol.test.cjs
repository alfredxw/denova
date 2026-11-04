const assert = require('node:assert/strict')
const { test } = require('node:test')
const { createServer } = require('node:http')
const { once } = require('node:events')
const { ORIGIN, forwardRequest } = require('../src/protocol.cjs')

test('proxy preserves bodies, streams SSE before completion and cancels the upstream', async t => {
  let received, disconnected
  const closed = new Promise(resolve => { disconnected = resolve })
  const server = createServer(async (request, response) => {
    let body = ''
    for await (const chunk of request) body += chunk
    received = { body, url: request.url, token: request.headers['x-denova-desktop'], origin: request.headers.origin }
    response.writeHead(200, { 'Content-Type': 'text/event-stream', 'Set-Cookie': 'secret=ignored' })
    response.write('data: first\n\n')
    response.on('close', disconnected)
  })
  server.listen(0, '127.0.0.1')
  await once(server, 'listening')
  t.after(() => { server.closeAllConnections(); server.close() })
  const controller = new AbortController()
  const response = await forwardRequest(new Request(ORIGIN + '/api/events?q=1', { method: 'POST', body: 'input', signal: controller.signal }), { url: `http://127.0.0.1:${server.address().port}`, token: 'private' })
  assert.equal(response.headers.get('set-cookie'), null)
  const reader = response.body.getReader()
  assert.equal(new TextDecoder().decode((await reader.read()).value), 'data: first\n\n')
  assert.deepEqual(received, { body: 'input', url: '/api/events?q=1', token: 'private', origin: ORIGIN })
  controller.abort()
  await assert.rejects(reader.read())
  await closed
})

test('proxy rejects foreign and opaque origins before touching the backend', async () => {
  for (const origin of ['https://foreign.invalid', 'null']) {
    assert.equal((await forwardRequest(new Request(ORIGIN + '/api/private', { headers: { origin } }), null)).status, 403)
  }
  assert.equal((await forwardRequest(new Request('https://denova.localhost:8443/api/private'), null)).status, 403)
  assert.equal((await forwardRequest(new Request(ORIGIN + '/'), null)).status, 503)
})

test('desktop HMR serves renderer assets without forwarding desktop credentials', async t => {
  const server = createServer((request, response) => {
    response.end(JSON.stringify({
      url: request.url,
      desktopToken: request.headers['x-denova-desktop'] ?? null,
      authorization: request.headers.authorization ?? null,
      cookie: request.headers.cookie ?? null,
    }))
  })
  server.listen(0, '127.0.0.1')
  await once(server, 'listening')
  t.after(() => { server.closeAllConnections(); server.close() })
  const options = { rendererOrigin: `http://127.0.0.1:${server.address().port}` }
  const request = new Request(ORIGIN + '/src/main.tsx?t=1', {
    headers: { 'X-Denova-Desktop': 'private', Authorization: 'Bearer private', Cookie: 'secret=private' },
  })
  const response = await forwardRequest(request, null, options)
  assert.equal(response.headers.get('Cross-Origin-Resource-Policy'), 'same-origin')
  assert.deepEqual(await response.json(), {
    url: '/src/main.tsx?t=1', desktopToken: null, authorization: null, cookie: null,
  })
  for (const path of ['/api', '/api/status']) {
    assert.equal((await forwardRequest(new Request(ORIGIN + path), null, options)).status, 503)
  }
})
