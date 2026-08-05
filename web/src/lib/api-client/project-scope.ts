/** Explicitly selects a global resource projection or one Project projection. */
export type ResourceTarget =
  | { kind: 'global' }
  | { kind: 'project'; projectId: string }

export const GLOBAL_RESOURCE_TARGET: ResourceTarget = Object.freeze({ kind: 'global' })

export function projectResourceTarget(projectId: string): ResourceTarget {
  const normalized = projectId.trim()
  if (!normalized) throw new Error('Project ID is required')
  return { kind: 'project', projectId: normalized }
}

export function resourceTargetKey(target: ResourceTarget): string {
  return target.kind === 'project' ? `project:${target.projectId}` : 'global'
}

/** Rejects a response that could otherwise write data into the wrong mounted Project. */
export function assertProjectScope(expected: string, actual: unknown, resource = 'Project response'): asserts actual is string {
  if (actual !== expected) {
    throw new Error(`${resource} scope mismatch: expected ${expected}, received ${typeof actual === 'string' && actual ? actual : '<empty>'}`)
  }
}

/** Builds the only client-authoritative Project API prefix. Content paths are
 * always resolved by the server from this stable identity. */
export function projectAPIPath(projectId: string, resourcePath = ''): string {
  const normalized = projectId.trim()
  if (!normalized) throw new Error('Project ID is required')
  const suffix = resourcePath ? `/${resourcePath.replace(/^\/+/, '')}` : ''
  return `/api/projects/${encodeURIComponent(normalized)}${suffix}`
}
