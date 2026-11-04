import { fetch as nativeFetch } from '@mygo-plugins/fetch'
import { desktopBridge } from './desktop'

// WKWebView custom-scheme tasks can omit POST bodies. The official plugin
// preserves bodies, streaming and cancellation; Go authenticates the caller
// and resolves this placeholder to the owned backend. Other URLs stay native.
export function installDesktopFetch() {
  if (!desktopBridge()) return
  const webviewFetch = window.fetch.bind(window)
  window.fetch = (input, init) => {
    const url = new URL(input instanceof Request ? input.url : String(input), location.href)
    if (url.origin !== location.origin || !url.pathname.startsWith('/api/')) return webviewFetch(input, init)
    const target = `http://mygo.localhost${url.pathname}${url.search}`
    return nativeFetch(input instanceof Request ? new Request(target, input) : target, init)
  }
}
