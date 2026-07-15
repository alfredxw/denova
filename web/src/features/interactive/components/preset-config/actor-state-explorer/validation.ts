import type { ExplorerProps } from './types'

export function isActorStateExplorerValueValid(value: ExplorerProps['value']) {
  const templateIds = new Set<string>()
  const poolIds = new Set<string>()

  for (const pool of value.trait_pools || []) {
    if (!pool.id || !pool.name || poolIds.has(pool.id)) return false
    poolIds.add(pool.id)
    const traitIds = new Set<string>()
    for (const trait of pool.traits || []) {
      if (!trait.id || !trait.name || traitIds.has(trait.id)) return false
      if (!Number.isFinite(trait.weight ?? 1) || (trait.weight ?? 1) <= 0) return false
      traitIds.add(trait.id)
    }
  }

  for (const tpl of value.templates || []) {
    if (!tpl.id || !tpl.name || templateIds.has(tpl.id)) return false
    templateIds.add(tpl.id)
    const fieldNames = new Set<string>()
    for (const field of tpl.fields || []) {
			const normalizedName = normalizeStateName(field.name)
			if (!normalizedName || actorStateFieldNameHasPathSeparator(field.name) || fieldNames.has(normalizedName)) return false
			fieldNames.add(normalizedName)
    }
    const rulePools = new Set<string>()
    for (const rule of tpl.trait_rules || []) {
      const pool = (value.trait_pools || []).find((candidate) => candidate.id === rule.pool_id)
      if (!pool || rulePools.has(rule.pool_id)) return false
      if (!Number.isInteger(rule.draw_count) || rule.draw_count < 1 || rule.draw_count > (pool.traits?.length || 0)) return false
      rulePools.add(rule.pool_id)
    }
  }
  for (const actor of value.initial_actors || []) {
    if (!actor.id || !actor.name || !actor.template_id || !templateIds.has(actor.template_id)) return false
  }
  return true
}

export function actorStateFieldNameHasPathSeparator(value: string | undefined) {
	return normalizeStateName(value).includes('/')
}

function normalizeStateName(value: string | undefined) {
	return (value || '').normalize('NFKC').trim().toLocaleLowerCase()
}
