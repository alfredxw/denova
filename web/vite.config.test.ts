import { Agent, createServer, get, type Server } from 'node:http'
import { once } from 'node:events'
import type { AddressInfo } from 'node:net'
import { mkdtemp, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { createServer as createViteServer } from 'vite'
import { expect, it } from 'vitest'
import config from './vite.config'

it('reuses browser and backend connections across API requests', async () => {
  let backendConnections = 0
  let frontendConnections = 0
  const backend = createServer((_request, response) => {
    response.setHeader('Content-Type', 'application/json')
    response.end(JSON.stringify({ content: 'Chapter content' }))
  })
  backend.on('connection', () => backendConnections++)
  backend.listen(0, '127.0.0.1')
  await once(backend, 'listening')

  const proxyOptions = config.server!.proxy!['/api']
  if (typeof proxyOptions === 'string') throw new Error('Expected API proxy options')
  const browserAgent = new Agent({ keepAlive: true })
  const cacheDir = await mkdtemp(join(tmpdir(), 'denova-proxy-test-'))
  let frontend: Server | undefined
  let vite: Awaited<ReturnType<typeof createViteServer>> | undefined
  try {
    vite = await createViteServer({
      configFile: false,
      cacheDir,
      appType: 'custom',
      optimizeDeps: { noDiscovery: true, include: [] },
      server: {
        middlewareMode: true,
        watch: null,
        proxy: {
          '/api': {
            ...proxyOptions,
            target: `http://127.0.0.1:${(backend.address() as AddressInfo).port}`,
          },
        },
      },
    })
    frontend = createServer(vite.middlewares)
    frontend.on('connection', () => frontendConnections++)
    frontend.listen(0, '127.0.0.1')
    await once(frontend, 'listening')
    const url = `http://127.0.0.1:${(frontend.address() as AddressInfo).port}/api/projects/book/files/file?path=chapter.md`

    for (let index = 0; index < 3; index++) {
      const result = await new Promise<{ connection?: string; body: string }>((resolve, reject) => {
        const request = get(url, { agent: browserAgent }, (response) => {
          let body = ''
          response.setEncoding('utf8')
          response.on('data', (chunk: string) => { body += chunk })
          response.on('end', () => resolve({ connection: response.headers.connection, body }))
          response.on('error', reject)
        })
        request.on('error', reject)
      })
      expect(JSON.parse(result.body)).toEqual({ content: 'Chapter content' })
      expect(result.connection).not.toBe('close')
    }
    expect(frontendConnections).toBe(1)
    expect(backendConnections).toBe(1)
  } finally {
    browserAgent.destroy()
    if (proxyOptions.agent instanceof Agent) proxyOptions.agent.destroy()
    frontend?.closeAllConnections()
    if (frontend) await new Promise<void>((resolve) => frontend!.close(() => resolve()))
    await vite?.close()
    backend.closeAllConnections()
    await new Promise<void>((resolve) => backend.close(() => resolve()))
    await rm(cacheDir, { recursive: true, force: true })
  }
})
