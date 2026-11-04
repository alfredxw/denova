import { once } from 'node:events'
import { createServer } from 'node:http'
import { fileURLToPath } from 'node:url'
import { createServer as createViteServer } from 'vite'

// The desktop development runner owns this server. The window keeps its desktop origin and connects
// directly to the loopback listener only for Vite's HMR WebSocket.
export async function startDesktopRenderer() {
  const host = '127.0.0.1'
  let vite
  const listener = createServer((request, response) => {
    if (vite) vite.middlewares(request, response)
    else response.writeHead(503).end()
  })
  await once(listener.listen(0, host), 'listening')
  const { port } = listener.address()
  try {
    vite = await createViteServer({
      root: fileURLToPath(new URL('../', import.meta.url)),
      mode: 'desktop',
      cacheDir: 'node_modules/.vite-desktop',
      server: {
        host, port, middlewareMode: true,
        hmr: { server: listener, protocol: 'ws', host, clientPort: port },
      },
    })
  } catch (error) {
    listener.close()
    throw error
  }
  return {
    origin: `http://${host}:${port}`,
    async close() {
      try {
        await vite.close()
      } finally {
        const closed = once(listener, 'close')
        listener.close()
        listener.closeAllConnections()
        await closed
      }
    },
  }
}
