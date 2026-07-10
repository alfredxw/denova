import type { ExplorerProps } from './types'

/**
 * Build path suggestions for StateOp path autocomplete.
 * Format: actors.{actor_id}.state.{field_path}
 */
export function buildPathSuggestions(value: ExplorerProps['value']): string[] {
  const suggestions: string[] = []
  const actors = value.initial_actors || []
  const templates = value.templates || []

  // Also suggest direct actor state paths
  for (const actor of actors) {
    suggestions.push(`actors.${actor.id}.state`)
  }

  // Build per-actor field paths
  for (const actor of actors) {
    const template = templates.find((t) => t.id === actor.template_id)
    if (!template) continue
    const fields = template.fields || []
    for (const field of fields) {
      suggestions.push(`actors.${actor.id}.state.${field.path}`)
    }
  }

  // Also suggest template-level paths without actor prefix
  for (const template of templates) {
    const fields = template.fields || []
    for (const field of fields) {
      suggestions.push(`${field.path}`)
    }
  }

  // Also suggest paths for all actors using wildcard
  for (const template of templates) {
    const fields = template.fields || []
    for (const field of fields) {
      suggestions.push(`actors.*.state.${field.path}`)
    }
  }

  return Array.from(new Set(suggestions))
}

/**
 * Build a map from path → field type for value editor type inference.
 */
export function buildPathTypeMap(value: ExplorerProps['value']): Record<string, string> {
  const map: Record<string, string> = {}
  const actors = value.initial_actors || []
  const templates = value.templates || []

  for (const actor of actors) {
    const template = templates.find((t) => t.id === actor.template_id)
    if (!template) continue
    const fields = template.fields || []
    for (const field of fields) {
      const path = `actors.${actor.id}.state.${field.path}`
      map[path] = field.type
    }
  }

  // Also add bare paths
  for (const template of templates) {
    const fields = template.fields || []
    for (const field of fields) {
      if (!map[field.path]) {
        map[field.path] = field.type
      }
    }
  }

  return map
}

/**
 * Build a map from path → field options (for enum fields).
 */
export function buildPathOptionsMap(value: ExplorerProps['value']): Record<string, string[]> {
  const map: Record<string, string[]> = {}
  const actors = value.initial_actors || []
  const templates = value.templates || []

  for (const actor of actors) {
    const template = templates.find((t) => t.id === actor.template_id)
    if (!template) continue
    const fields = template.fields || []
    for (const field of fields) {
      if (field.type === 'enum' && field.options && field.options.length > 0) {
        const path = `actors.${actor.id}.state.${field.path}`
        map[path] = field.options
      }
    }
  }

  // Also add bare paths
  for (const template of templates) {
    const fields = template.fields || []
    for (const field of fields) {
      if (field.type === 'enum' && field.options && field.options.length > 0) {
        if (!map[field.path]) {
          map[field.path] = field.options
        }
      }
    }
  }

  return map
}
