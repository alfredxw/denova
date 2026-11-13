import i18n from '@/i18n'
import { APIError, jsonHeaders, requestJSON } from '@/lib/api-client/client'
import { errorMessage } from '@/lib/error-diagnostics'
import { remoteAccessIsLocal } from '@/features/remote-access/api'

export type PackageKind = 'plugin' | 'game'
export type ContributionContext = 'writing' | 'game' | 'general'
export type LocalizedText = { 'zh-CN': string; 'en-US': string }
export type ReleaseRef = {
  package: { kind: PackageKind; id: string }
  releaseId: string
}
export interface PageContribution { id: string; titleKey: string; viewId: string; agent?: { instructions: string; modelSlot: string; tools?: string[]; toolsets?: string[] } }
export interface Contributions {
  pages?: PageContribution[]
  tools?: { id: string; definition: string; agentContexts?: ContributionContext[] }[]
  toolsets?: { id: string; tools: string[] }[]
  commands?: { id: string; titleKey: string; contexts: ContributionContext[]; target: { kind: 'tool' | 'panel'; id: string } }[]
  panels?: { id: string; titleKey: string; contexts: ContributionContext[]; viewId: string }[]
}
export interface Manifest {
  id: string
  version: string
  apiMajor: number
  minHostVersion: string
  views?: { id: string; source: { kind: 'static' | 'backend'; path: string } }[]
  name: LocalizedText
  description?: LocalizedText
  runtime?: { backend?: unknown }
  settings?: { schema: string; defaults: string; uiSchema?: string }
  projectContent?: string[]
  modelSlots?: {
    id: string
    titleKey: string
    kind: string
    required: boolean
  }[]
  permissions: { required: string[]; optional: string[] }
  requires?: {
    pluginId: string
    versionRange: string
    contributions: string[]
  }[]
  contributes?: Contributions
  definitions?: Contributions & { agents?: { id: string; definition: string }[] }
  game?: {
    /** Optional distributed raster asset; only the host cover endpoint serves it. */
    cover?: string
    setup?: { schema: string; defaults: string; uiSchema?: string }
    viewId: string
    storage: { kind: 'self' | 'story'; saveFormat?: string }
    story?: { modelSlot: string }
    uses?: { agents?: string[]; toolsets?: string[] }
  }
}
export interface Release {
  ref: ReleaseRef
  manifest: Manifest
  digest: string
  installedAt: string
  grants?: string[]
}
export interface Installed {
  source?: GitHubSource
  id: string
  enabled: boolean
  removed?: boolean
  currentRelease: string
  grants: string[]
  releases: Release[]
}
export interface CatalogEntry extends Installed {
  kind: PackageKind
  unavailableReason?: string
}
export const BUILTIN_GAME_ID = 'builtin.story'
export interface GamePreferences { defaultGameId: string }

export interface Candidate {
  source?: GitHubSource
  candidateId: string
  kind: PackageKind
  manifest: Manifest
  digest: string
  files: string[]
  bytes: number
}
export interface Development {
  source?: GitHubSource
  developmentId: string
  kind: PackageKind
  projectId: string
  relativePath: string
}
/** Upstream ref and downloaded commit; paths are repository-relative. */
export interface GitHubSource {
  url: string
  ref: string
  path: string
  commit?: string
}
export interface GitHubUpdate {
  status: 'current' | 'available'
  source: GitHubSource
}
/** Live source metadata, never a second persisted copy of a manifest. */
export interface DevelopmentSource extends Development {
  projectName: string
  manifest?: Manifest
  messageKey?: string
}
export interface Instance {
  storyId?: string
  instanceId: string
  gameId: string
  releaseId: string
  title: string
  projectId?: string
  dependencies: { pluginId: string; releaseId: string }[]
  models: Record<string, string>
  setup: Record<string, unknown>
  preview: boolean
  createdAt: string
}
export interface RuntimeSnapshot {
  id: string
  status: string
  viewUrl?: string
  connection: { baseUrl: string; token: string; consumerId?: string }
  context: {
    source: ReleaseRef
    scope: { kind: string; projectId?: string; instanceId?: string; storyId?: string; branchId?: string }
    locale: string
    theme: string
    environment: string
    settings: Record<string, unknown>
    setup?: Record<string, unknown>
  }
}
export const managementBase = '/api/platform/manage'

/**
 * Host-bound management actions mirror the backend classification in
 * internal/api/middleware.go: runtimes listen on loopback, package installation
 * runs extension code on the Denova host, and previews resolve host paths.
 * Remote browsers fail fast with a clear reason instead of a generic 403.
 */
export function hostOnlyManagement(path: string, method = 'GET'): boolean {
  const normalized = method.toUpperCase()
  if (normalized === 'GET' || normalized === 'HEAD' || normalized === 'OPTIONS') return false
  const parts = path.split('?')[0].split('/').filter(Boolean)
  const [section, , third, , fifth] = parts
  switch (section) {
    case 'game-preferences':
      return !(parts.length === 1 && normalized === 'PATCH')
    case 'instances':
      return !((parts.length === 2 && (normalized === 'PATCH' || normalized === 'DELETE'))
        || (parts.length === 3 && third === 'stop' && normalized === 'POST'))
    case 'runtimes':
      return !((parts.length === 3 && third === 'stop' && normalized === 'POST')
        || (parts.length === 4 && third === 'consumers' && normalized === 'DELETE'))
    case 'projects':
      return !((parts.length === 3 && third === 'extensions' && normalized === 'PUT')
        || (parts.length === 5 && third === 'plugins' && fifth === 'content' && normalized === 'POST'))
    default:
      return true
  }
}

function localHostOnlyError(): APIError {
  const message = i18n.t('platform.errors.LOCAL_HOST_ONLY')
  return new APIError(message, {
    status: 403,
    code: 'LOCAL_HOST_ONLY',
    payload: { code: 'LOCAL_HOST_ONLY', messageKey: 'platform.errors.LOCAL_HOST_ONLY' },
  })
}

export interface ConfigurationProblem {
  messageKey: string
  fields?: { path: string[]; keyword: string }[]
}
export interface ConfigurationDocument {
  revision: string
  releaseId: string
  form: { schema: import('@rjsf/utils').RJSFSchema; uiSchema: import('@rjsf/utils').UiSchema; defaults: Record<string, unknown> } | null
  overrides: Record<string, unknown>
  values: Record<string, unknown>
  problem?: ConfigurationProblem
}
export interface RuntimeSetupDocument extends ConfigurationDocument {
  models: { key: string; kind: string; required: boolean }[]
}

export interface PluginAction {
  id: string
  kind: 'command' | 'panel'
  title: LocalizedText
  target: { kind: 'tool' | 'panel'; id: string }
  form?: NonNullable<ConfigurationDocument['form']>
}
export interface ProjectPlugin {
  pages: PluginPage[]
  id: string
  releaseId: string
  name: LocalizedText
  enabled: boolean
  problem?: ConfigurationProblem
  models: RuntimeSetupDocument['models']
  actions: PluginAction[]
}
export interface ProjectExtensionConfiguration {
  revision: string
  extensions: { disabledPlugins: string[]; models: Record<string, string> }
}

export function management<T>(
  path: string,
  method = 'GET',
  body?: unknown,
): Promise<T> {
  if (!remoteAccessIsLocal() && hostOnlyManagement(path, method)) return Promise.reject(localHostOnlyError())
  return requestJSON<T>(managementBase + path, {
    method,
    ...(body === undefined
      ? {}
      : { headers: jsonHeaders, body: JSON.stringify(body) }),
  })
}
export async function consumer<T>(
  runtime: RuntimeSnapshot,
  path: string,
  method = 'GET',
  body?: unknown,
  signal?: AbortSignal,
): Promise<T> {
  return requestJSON<T>(runtime.connection.baseUrl + path, {
    method,
    signal,
    headers: {
      ...jsonHeaders,
      Authorization: `Bearer ${runtime.connection.token}`,
      ...(runtime.connection.consumerId ? { 'X-Denova-Consumer': runtime.connection.consumerId } : {}),
    },
    ...(body === undefined ? {} : { body: JSON.stringify(body) }),
  })
}
export function platformError(error: unknown): string {
  const key =
    error instanceof APIError && typeof error.payload.messageKey === 'string'
      ? error.payload.messageKey
      : 'platform.errors.RUNTIME_FAILED'
  let summary = i18n.exists(key)
    ? i18n.t(key)
    : i18n.t('platform.errors.RUNTIME_FAILED')
  if (error instanceof APIError) {
    if (error.code === 'GITHUB_RATE_LIMIT' && typeof error.payload.retryAt === 'string') {
      summary = i18n.t('platform.errors.GITHUB_RATE_LIMIT_UNTIL', {
        time: new Date(error.payload.retryAt).toLocaleString(i18n.resolvedLanguage),
      })
    }
    return errorMessage({
      summary, code: error.code, status: error.status, requestID: error.requestID,
      details: { ...error.details, detail: error.details?.detail || error.payload.diagnostic },
    })
  }
  return errorMessage({ summary, details: { detail: error instanceof Error ? error.message : undefined } })
}
export function localized(text: LocalizedText | undefined, language: string) {
  return text?.[language.startsWith('zh') ? 'zh-CN' : 'en-US'] ?? ''
}

/** Best-effort release also survives a normal browser page unload. */
export async function releasePluginConsumer(runtime: RuntimeSnapshot) {
  if (runtime.connection.consumerId) await requestJSON(managementBase + `/runtimes/${runtime.id}/consumers/${runtime.connection.consumerId}`, { method: 'DELETE', keepalive: true })
}

export interface PluginPage { id: string; pluginId: string; releaseId: string; title: LocalizedText; agent: boolean }
