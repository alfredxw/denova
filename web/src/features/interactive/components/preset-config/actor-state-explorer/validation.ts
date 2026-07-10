import type { StateOp } from '../../../types'
import type { ExplorerProps } from './types'

const STATE_OP_TYPES = new Set(['set', 'merge', 'push', 'pull', 'inc', 'unset'])

export function isActorStateExplorerValueValid(value: ExplorerProps['value']) {
  for (const tpl of value.templates || []) {
    if (!tpl.id || !tpl.name) return false
    for (const field of tpl.fields || []) {
      if (!field.path || !field.name) return false
    }
  }
  for (const actor of value.initial_actors || []) {
    if (!actor.id || !actor.name || !actor.template_id) return false
  }
  for (const op of value.initial_state_ops || []) {
    if (!isStateOpValid(op)) return false
  }
  for (const pool of value.trait_pools || []) {
    for (const trait of pool.traits || []) {
      for (const op of trait.ops || []) {
        if (!isStateOpValid(op)) return false
      }
    }
  }
  return true
}

function isStateOpValid(op: StateOp) {
  const opName = String(op.op || '').trim()
  const path = String(op.path || '').trim()
  if (!STATE_OP_TYPES.has(opName)) return false
  if (!path || path.startsWith('.') || path.endsWith('.') || path.includes('..')) return false
  return true
}
