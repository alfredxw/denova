import type { TFunction } from 'i18next'
import type { ActorStateField, ActorStateInitialActor, ActorTraitDefinition, ActorTraitPool } from '../../../types'
import type { ExplorerProps, TreeNode } from './types'

/** Build a tree view of the actor state system for navigation. */
export function buildStateTree(value: ExplorerProps['value'], t: TFunction): TreeNode[] {
  const templates = value.templates || []
  const initialActors = value.initial_actors || []
  const pools = value.trait_pools || []

  const root: TreeNode[] = []

  // ── Templates group ──────────────────────────────────────────────
  const templateNodes: TreeNode[] = templates.map((template, tIndex) => {
    const fields = template.fields || []
    const fieldNodes: TreeNode[] = fields.map((field, fIndex) => ({
      id: fieldNodeId(template.id, field, fIndex),
      kind: 'field' as const,
			label: field.name || t('settingPanel.actorState.explorer.fieldFallback', { count: fIndex + 1 }),
			subtitle: field.type,
      badge: field.visibility ? visibilityBadge(field.visibility, t) : undefined,
      selectable: true,
      children: [],
      data: { kind: 'field' as const, field, fieldIndex: fIndex, template, templateIndex: tIndex },
    }))

    return {
      id: templateNodeId(template, tIndex),
      kind: 'template' as const,
      label: template.name || template.id || t('settingPanel.actorState.explorer.templateFallback', { count: tIndex + 1 }),
      subtitle: t('settingPanel.actorState.explorer.fieldCount', { count: fields.length }),
      selectable: true,
      children: fieldNodes,
      data: { kind: 'template' as const, template, index: tIndex },
    }
  })

  root.push({
    id: 'group:templates',
    kind: 'group',
    label: t('settingPanel.actorState.templates'),
    badge: `${templates.length}`,
    selectable: false,
    children: templateNodes,
  })

  // ── Trait library ────────────────────────────────────────────────
  const poolNodes: TreeNode[] = pools.map((pool, pIndex) => {
    const traits = pool.traits || []
    const traitNodes: TreeNode[] = traits.map((trait, tIndex) => ({
      id: traitNodeId(pool, trait, tIndex),
      kind: 'trait' as const,
      label: trait.name || trait.id || t('settingPanel.actorState.explorer.traitFallback', { count: tIndex + 1 }),
      subtitle: trait.summary || t('settingPanel.actorState.explorer.weight', { count: trait.weight ?? 1 }),
      badge: trait.visibility ? visibilityBadge(trait.visibility, t) : undefined,
      selectable: true,
      children: [],
      data: { kind: 'trait' as const, trait, traitIndex: tIndex, pool, poolIndex: pIndex },
    }))

    return {
      id: poolNodeId(pool, pIndex),
      kind: 'pool' as const,
      label: pool.name || pool.id || t('settingPanel.actorState.explorer.poolFallback', { count: pIndex + 1 }),
      subtitle: pool.description || t('settingPanel.actorState.explorer.traitCount', { count: traits.length }),
      badge: `${traits.length}`,
      selectable: true,
      children: traitNodes,
      data: { kind: 'pool' as const, pool, poolIndex: pIndex },
    }
  })

  root.push({
    id: 'group:traits',
    kind: 'trait-library',
    label: t('settingPanel.actorState.explorer.traitLibrary'),
    badge: `${pools.length}`,
    selectable: false,
    children: poolNodes,
  })

  // ── Initial actor instances ──────────────────────────────────────
  const actorNodes: TreeNode[] = initialActors.map((actor, actorIndex) => {
    const template = templates.find((item) => item.id === actor.template_id)
    return {
      id: actorNodeId(actor, actorIndex),
      kind: 'actor' as const,
      label: actor.name || actor.id || t('settingPanel.actorState.explorer.actorFallback', { count: actorIndex + 1 }),
      subtitle: actor.role || template?.name || actor.template_id,
      selectable: true,
      children: [],
      data: { kind: 'actor' as const, actor, actorIndex, template },
    }
  })
  root.push({
    id: 'group:actors',
    kind: 'actors-group',
    label: t('settingPanel.actorState.initialActors'),
    badge: `${initialActors.length}`,
    selectable: false,
    children: actorNodes,
  })

  return root
}

export function templateNodeId(template: { id?: string }, index: number): string {
  return `template:${template.id || index}`
}

export function fieldNodeId(templateId: string, field: ActorStateField, index: number): string {
	void field
	return `field:${templateId}:${index}`
}

export function actorNodeId(actor: ActorStateInitialActor, index: number): string {
  return `actor:${actor.id || actor.template_id || 'actor'}-${index}`
}

export function poolNodeId(pool: ActorTraitPool, index: number): string {
  return `pool:${pool.id || index}`
}

export function traitNodeId(pool: ActorTraitPool, trait: ActorTraitDefinition, index: number): string {
  return `trait:${pool.id || 'pool'}:${trait.id || index}`
}

function visibilityBadge(visibility: string, t: TFunction): string {
  switch (visibility) {
    case 'visible': return t('settingPanel.actorState.explorer.visible')
    case 'hidden': return t('settingPanel.actorState.explorer.hidden')
    case 'spoiler': return t('settingPanel.actorState.explorer.spoiler')
    default: return visibility
  }
}

/** Find a node by id in the tree (depth-first). */
export function findNode(nodes: TreeNode[], id: string): TreeNode | null {
  for (const node of nodes) {
    if (node.id === id) return node
    if (node.children.length > 0) {
      const found = findNode(node.children, id)
      if (found) return found
    }
  }
  return null
}

/** Get the breadcrumb path (array of node labels) from root to the node with given id. */
export function getBreadcrumb(nodes: TreeNode[], id: string): TreeNode[] {
  const path: TreeNode[] = []
  dfs(nodes, id, path)
  return path
}

function dfs(nodes: TreeNode[], targetId: string, path: TreeNode[]): boolean {
  for (const node of nodes) {
    path.push(node)
    if (node.id === targetId) return true
    if (node.children.length > 0 && dfs(node.children, targetId, path)) return true
    path.pop()
  }
  return false
}

/** Get all ancestor group ids that should be expanded to show this node. */
export function getExpandedAncestors(nodes: TreeNode[], id: string): Set<string> {
  const path = getBreadcrumb(nodes, id)
  const expanded = new Set<string>()
  for (const node of path) {
    if (node.children.length > 0) expanded.add(node.id)
  }
  return expanded
}

/** Get default expanded set: top-level groups + first template + first pool. */
export function getDefaultExpanded(nodes: TreeNode[]): Set<string> {
  const expanded = new Set<string>()
  for (const node of nodes) {
    if (node.children.length > 0) {
      expanded.add(node.id)
      // Expand first child of each group
      const firstChild = node.children[0]
      if (firstChild && firstChild.children.length > 0) {
        expanded.add(firstChild.id)
      }
    }
  }
  return expanded
}

/** Find the first selectable node in the tree. */
export function findFirstSelectable(nodes: TreeNode[]): TreeNode | null {
  for (const node of nodes) {
    if (node.selectable) return node
    if (node.children.length > 0) {
      const found = findFirstSelectable(node.children)
      if (found) return found
    }
  }
  return null
}
