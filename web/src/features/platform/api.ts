import i18n from '@/i18n'
import { APIError, jsonHeaders, requestJSON } from '@/lib/api-client/client'

export type PackageKind = 'plugin' | 'game'
export type LocalizedText = { 'zh-CN': string; 'en-US': string }
export type ReleaseRef = {
  package: { kind: PackageKind; id: string }
  releaseId: string
}
export interface Contributions {
  tools?: { id: string; definition: string }[]
  toolsets?: { id: string; tools: string[] }[]
}
export interface Manifest {
  id: string
  version: string
  apiMajor: number
  name: LocalizedText
  description?: LocalizedText
  runtime?: { backend?: unknown }
  settings?: { schema: string; defaults: string; uiSchema?: string }
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
  connection: { baseUrl: string; token: string }
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

export interface ConfigurationProblem {
  messageKey: string
  packageId?: string
  version?: string
  fields?: { path: string[]; keyword: string }[]
}
export interface ConfigurationDocument {
  revision: string
  releaseId: string
  form: { schema: import('@rjsf/utils').RJSFSchema; uiSchema: import('@rjsf/utils').UiSchema; defaults: Record<string, unknown> } | null
  overrides: Record<string, unknown>
  values: Record<string, unknown>
  toml: string
  problem?: ConfigurationProblem
}
export function management<T>(
  path: string,
  method = 'GET',
  body?: unknown,
): Promise<T> {
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
): Promise<T> {
  return requestJSON<T>(runtime.connection.baseUrl + path, {
    method,
    headers: {
      ...jsonHeaders,
      Authorization: `Bearer ${runtime.connection.token}`,
    },
    ...(body === undefined ? {} : { body: JSON.stringify(body) }),
  })
}
export function platformError(error: unknown): string {
  const key =
    error instanceof APIError && typeof error.payload.messageKey === 'string'
      ? error.payload.messageKey
      : 'platform.errors.RUNTIME_FAILED'
  return i18n.exists(key)
    ? i18n.t(key)
    : i18n.t('platform.errors.RUNTIME_FAILED')
}
export function localized(text: LocalizedText | undefined, language: string) {
  return text?.[language.startsWith('zh') ? 'zh-CN' : 'en-US'] ?? ''
}
