import { vi } from 'vitest'

type RouteContext = {
  request: Request
  params: Record<string, string>
}

type RouteResolver = (context: RouteContext) => Response | Promise<Response>
type PathMatcher = (pathname: string) => Record<string, string> | null

export interface ApiRoute {
  method: string
  match: (pathname: string) => Record<string, string> | null
  resolve: RouteResolver
}

function compilePath(pattern: string): PathMatcher {
  const parameterNames: string[] = []
  const source = pattern
    .split('/')
    .map((segment) => {
      if (!segment.startsWith(':')) return segment.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
      parameterNames.push(segment.slice(1))
      return '([^/]+)'
    })
    .join('/')
  const expression = new RegExp(`^${source}$`)
  return (pathname: string) => {
    const match = expression.exec(pathname)
    if (!match) return null
    return Object.fromEntries(parameterNames.map((name, index) => [name, decodeURIComponent(match[index + 1])]))
  }
}

function route(method: string, pattern: string, resolve: RouteResolver): ApiRoute {
  return { method, match: compilePath(pattern), resolve }
}

export const apiRoute = {
  get(pattern: string, resolve: RouteResolver): ApiRoute {
    return route('GET', pattern, resolve)
  },
  post(pattern: string, resolve: RouteResolver): ApiRoute {
    return route('POST', pattern, resolve)
  },
  put(pattern: string, resolve: RouteResolver): ApiRoute {
    return route('PUT', pattern, resolve)
  },
  patch(pattern: string, resolve: RouteResolver): ApiRoute {
    return route('PATCH', pattern, resolve)
  },
  delete(pattern: string, resolve: RouteResolver): ApiRoute {
    return route('DELETE', pattern, resolve)
  },
}

export function jsonResponse(body: unknown, init: ResponseInit = {}): Response {
  const headers = new Headers(init.headers)
  if (!headers.has('Content-Type')) headers.set('Content-Type', 'application/json')
  return new Response(JSON.stringify(body), { ...init, headers })
}

export function textResponse(body: string, init: ResponseInit = {}): Response {
  return new Response(body, init)
}

export function installApiMock(...initialRoutes: ApiRoute[]) {
  let routes = [...initialRoutes]
  const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const request = input instanceof Request
      ? input
      : new Request(new URL(String(input), 'http://localhost'), init)
    const pathname = new URL(request.url).pathname
    for (const candidate of routes) {
      if (candidate.method !== request.method) continue
      const params = candidate.match(pathname)
      if (params) return candidate.resolve({ request, params })
    }
    throw new Error(`Unhandled API request: ${request.method} ${pathname}`)
  })
  vi.stubGlobal('fetch', fetchMock)
  return {
    fetchMock,
    use: (...overrides: ApiRoute[]) => {
      routes = [...overrides, ...routes]
    },
  }
}
