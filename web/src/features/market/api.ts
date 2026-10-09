import { fetchAPI, jsonHeaders, requestJSON } from '@/lib/api-client/client'
import type { CharacterCardImportResult } from '@/lib/api'
import type { Candidate } from '@/features/platform/api'
import type { GameCreationDefaults, GameDefaultField } from '@/features/interactive/game-creation-defaults'

export interface PackageGameDefaults extends Omit<GameCreationDefaults, 'default_background'> {
  default_background?: { resource_id: string; item_id: string; asset_path: string }
}

export const submissionURL =
  'https://github.com/alfredxw/denova-index/issues/new?template=package.yml'
export const resourceKinds = [
  'preset.narrative',
  'preset.image',
  'preset.game_planning',
  'preset.events',
  'preset.rules',
  'preset.actor_state',
  'style.reference',
  'skill',
  'lore.collection',
  'game.openings',
  'project.cover',
  'project.creator',
  'extension.plugin',
  'extension.game',
] as const
export type ResourceKind = (typeof resourceKinds)[number]
export const isCollectionKind = (kind: ResourceKind) => kind === 'lore.collection' || kind === 'game.openings'
export interface Source {
  kind: 'file' | 'github' | 'https_zip'
  url?: string
  ref?: string
  path?: string
  commit?: string
  filename?: string
}
export interface PackageTranslation {
  name?: string
  description?: string
  usage?: string
  compatibility?: string
}
export interface PackageInfo {
  id: string
  name: string
  description?: string
  version?: string
  author?: string
  min_denova_version?: string
  locale?: string
  translations?: Record<string, PackageTranslation>
  tags?: string[]
  cover?: string
  usage?: string
  compatibility?: string
  updated_at?: string
}
export interface MarketEntry {
  id: string
  name: Record<string, string>
  description: Record<string, string>
  author: string
  format: string
  kinds: ResourceKind[]
  tags: string[]
  source: Source
  updated_at: string
  featured?: boolean
  compatibility?: Record<string, string>
  usage?: Record<string, string>
}
export interface Catalog {
  registry_url?: string
  schema_version: number
  entries: MarketEntry[]
  fetched_at?: string
  stale?: boolean
  error_key?: string
}
export interface PreviewResource {
  item_count?: number
  id: string
  kind: ResourceKind
  path: string
  requires?: string[]
  name: string
  description?: string
  extension?: Candidate
  digest: string
}
export interface PackagePreview {
  game_defaults?: PackageGameDefaults
  candidate_id: string
  package: PackageInfo
  format: string
  resources: PreviewResource[]
}
export interface Preview {
  character?: CharacterCardImportResult
  preview_id: string
  source: Source
  expires_at: string
  candidates: PackagePreview[]
}
// A selection always refers to one candidate in the same frozen preview.
export function discardPreview(preview: Preview) {
  void exchange(`/previews/${preview.preview_id}`, undefined, 'DELETE').catch((error) => {
    console.error('[market] failed to release preview', error)
  })
}
export interface LocalRef {
  kind: ResourceKind
  scope: string
  id: string
  project_id?: string
}
export interface Binding {
  upstream_removed?: boolean
  resource_id: string
  local: LocalRef
  ownership: string
  source_digest: string
}
export interface Installation {
  game_defaults?: GameCreationDefaults
  installation_id: string
  package: PackageInfo
  source: Source
  project_id?: string
  tracking: string
  update_mode: string
  bindings: Binding[]
  updated_at: string
  remote_state?: string
  local_state?: string
}
export interface PlanItem {
  resource_id: string
  name: string
  local: LocalRef
  action: string
  extension?: Candidate
  grants?: string[]
}
export interface UpdateItem {
  resource_id: string
  member_id?: string
  name: string
  state: 'create' | 'update' | 'unchanged' | 'keep' | 'conflict' | 'blocked' | 'upstream_removed'
  conflict?: boolean
  resolution?: string
}
export interface Plan {
  updates?: UpdateItem[]
  game_defaults_before?: GameCreationDefaults
  game_defaults_applied?: GameCreationDefaults
  plan_id: string
  items: PlanItem[]
  installation: Installation
  expires_at: string
}
export interface PlanRequest {
  shared_resources?: 'reuse' | 'copy'
  resolutions?: Record<string, Record<string, string>>
  game_defaults_fields?: GameDefaultField[]
  preview_id: string
  candidate_id: string
  resources: string[]
  project_id?: string
  skill_scope?: string
  installation_id?: string
  grants?: Record<string, string[]>
  names?: Record<string, string>
  replace_modified?: boolean
  update_mode: string
}
export interface ExportResource {
  item_count?: number
  local: LocalRef
  name: string
  description?: string
}
export const exchange = <T>(
  path: string,
  body?: unknown,
  method = body === undefined ? 'GET' : 'POST',
) =>
  requestJSON<T>(`/api/resource-exchange${path}`, {
    method,
    ...(body === undefined
      ? {}
      : { headers: jsonHeaders, body: JSON.stringify(body) }),
  })
export async function previewSource(source: Source | File | { kind: 'directory'; directory: string }) {
  if (source instanceof File) {
    const body = new FormData()
    body.set('file', source)
    return requestJSON<Preview>('/api/resource-exchange/previews', {
      method: 'POST',
      body,
    })
  }
  return exchange<Preview>('/previews', source)
}
export function getCatalog(refresh = false): Promise<Catalog> {
  return requestJSON<Catalog>(
    `/api/resource-market/catalog${refresh ? '/refresh' : ''}`,
    { method: refresh ? 'POST' : 'GET', suppressBackendUnavailableToast: true },
  )
}
export interface ExportRequest {
  package: PackageInfo
  resources: LocalRef[]
  native?: boolean
  installation_id?: string
}
export interface ExportDefinition extends ExportRequest {
  revision: string
}
export interface ExportPlan {
  export_id: string
  package: PackageInfo
  resources: {
    id: string
    kind: ResourceKind
    path: string
    requires?: string[]
  }[]
  files: number
  bytes: number
  expires_at: string
}
export async function downloadExport(plan: ExportPlan) {
  const response = await fetchAPI(
    `/api/resource-exchange/exports/${plan.export_id}/download`,
  )
  const url = URL.createObjectURL(await response.blob())
  const a = document.createElement('a')
  a.href = url
  a.download = `${plan.package.id || 'denova-resources'}.zip`
  a.click()
  setTimeout(() => URL.revokeObjectURL(url), 1000)
}
export function localized(value: Record<string, string>, language: string) {
  return (
    value[language] ||
    value[language.startsWith('zh') ? 'zh-CN' : 'en-US'] ||
    Object.values(value)[0] ||
    ''
  )
}
export function dependencySelection(
  resources: PreviewResource[],
  ids: string[],
): string[] {
  const selected = new Set<string>()
  const visit = (id: string) => {
    if (selected.has(id)) return
    selected.add(id)
    resources.find((resource) => resource.id === id)?.requires?.forEach(visit)
  }
  ids.forEach(visit)
  return [...selected]
}
