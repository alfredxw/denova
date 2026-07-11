import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import type { ActorStateField, ActorStateInitialActor, ActorStateTemplate, ActorTraitDefinition, ActorTraitPool } from '../../../types'
import type { ExplorerProps, TreeNode } from './types'
import { useExplorerSelection } from './use-explorer-selection'
import { getExpandedAncestors } from './build-tree'
import { StateTreeNavigator } from './tree/StateTreeNavigator'
import { StateDetailArea } from './detail/StateDetailArea'
import { isActorStateExplorerValueValid } from './validation'
import { cloneWithNewId, nextPresetId } from '../utils'

export function ActorStateExplorer({ value, onChange, onValidityChange, layout = 'card' }: ExplorerProps & { layout?: 'card' | 'attached' }) {
  const { t } = useTranslation()
  const [navigatorOpen, setNavigatorOpen] = useState(false)
  const {
    tree,
    selectedId,
    selectedNode,
    expandedIds,
    toggleExpanded,
    expandAncestors,
    selectNode,
    remapNodeId,
  } = useExplorerSelection(value)

  const selectAndReveal = useCallback((id: string) => {
    selectNode(id)
    const ancestors = getExpandedAncestors(tree, id)
    const ancestorIds = Array.from(ancestors)
    if (ancestorIds.length > 0) {
      expandAncestors(ancestorIds)
    }
  }, [tree, selectNode, expandAncestors])

  const handleSelect = useCallback((id: string) => {
    selectAndReveal(id)
    setNavigatorOpen(false)
  }, [selectAndReveal])

  // ── Add handlers ─────────────────────────────────────────────────

  const handleAddTemplate = useCallback(() => {
    const newTemplate: ActorStateTemplate = {
      id: nextPresetId('tpl'),
      name: t('settingPanel.actorState.explorer.newTemplate', { count: (value.templates?.length || 0) + 1 }),
      description: '',
      fields: [],
      trait_rules: [],
    }
    onChange({
      ...value,
      templates: [...(value.templates || []), newTemplate],
    })
    // Select the new template
    setTimeout(() => handleSelect(`template:${newTemplate.id}`), 0)
  }, [handleSelect, onChange, t, value])

  const handleAddField = useCallback((templateId: string) => {
    const templates = [...(value.templates || [])]
    const tIndex = templates.findIndex((t) => t.id === templateId)
    if (tIndex < 0) return
    const tpl = { ...templates[tIndex] }
    const fields = [...(tpl.fields || [])]
    const newField: ActorStateField = {
      name: t('settingPanel.actorState.explorer.newField', { count: fields.length + 1 }),
      type: 'string',
      visibility: 'visible',
      order: fields.length,
    }
    fields.push(newField)
    tpl.fields = fields
    templates[tIndex] = tpl
    onChange({ ...value, templates })
    setTimeout(() => handleSelect(`field:${templateId}:${fields.length - 1}`), 0)
  }, [handleSelect, onChange, t, value])

  const handleAddActor = useCallback((templateId: string) => {
    const actors = [...(value.initial_actors || [])]
    const effectiveTemplateId = templateId || value.templates?.[0]?.id || ''
    if (!effectiveTemplateId) return
    const newActor: ActorStateInitialActor = {
      id: nextPresetId('actor'),
      name: t('settingPanel.actorState.explorer.newActor', { count: actors.length + 1 }),
      template_id: effectiveTemplateId,
      role: 'supporting',
      state: {},
    }
    actors.push(newActor)
    onChange({ ...value, initial_actors: actors })
    setTimeout(() => handleSelect(`actor:${newActor.id}-${actors.length - 1}`), 0)
  }, [handleSelect, onChange, t, value])

  const handleAddPool = useCallback(() => {
    const pools = [...(value.trait_pools || [])]
    const newPool: ActorTraitPool = {
      id: nextPresetId('pool'),
      name: t('settingPanel.actorState.explorer.newPool', { count: pools.length + 1 }),
      description: '',
      traits: [],
    }
    pools.push(newPool)
    onChange({ ...value, trait_pools: pools })
    setTimeout(() => handleSelect(`pool:${newPool.id}`), 0)
  }, [handleSelect, onChange, t, value])

  const handleAddTrait = useCallback((poolId: string) => {
    const pools = [...(value.trait_pools || [])]
    const pIndex = pools.findIndex((p) => p.id === poolId)
    if (pIndex < 0) return
    const pool = { ...pools[pIndex] }
    const traits = [...(pool.traits || [])]
    const newTrait: ActorTraitDefinition = {
      id: nextPresetId('trait'),
      name: t('settingPanel.actorState.explorer.newTrait', { count: traits.length + 1 }),
      weight: 1,
      visibility: 'visible',
    }
    traits.push(newTrait)
    pool.traits = traits
    pools[pIndex] = pool
    onChange({ ...value, trait_pools: pools })
    setTimeout(() => handleSelect(`trait:${poolId}:${newTrait.id || ''}`), 0)
  }, [handleSelect, onChange, t, value])

  // ── Duplicate handler ────────────────────────────────────────────

  const handleDuplicateNode = useCallback((node: TreeNode) => {
    const data = node.data
    if (!data) return

    switch (data.kind) {
      case 'template': {
        const cloned = cloneWithNewId(data.template, 'tpl')
				cloned.fields = (data.template.fields || []).map((field) => {
					const next = { ...field }
					delete next.id
					delete next.path
					return next
				})
        const templates = [...(value.templates || [])]
        templates.splice(data.index + 1, 0, cloned)
        onChange({ ...value, templates })
        setTimeout(() => handleSelect(`template:${cloned.id}`), 0)
        break
      }
      case 'actor': {
        const cloned = cloneWithNewId(data.actor, 'actor')
        cloned.state = { ...(data.actor.state || {}) }
        const actors = [...(value.initial_actors || [])]
        actors.splice(data.actorIndex + 1, 0, cloned)
        onChange({ ...value, initial_actors: actors })
        setTimeout(() => handleSelect(`actor:${cloned.id}-${data.actorIndex + 1}`), 0)
        break
      }
      case 'pool': {
        const cloned: ActorTraitPool = {
          ...cloneWithNewId(data.pool, 'pool'),
          traits: (data.pool.traits || []).map((t) => ({
            ...t,
            id: nextPresetId('trait'),
          })),
        }
        const pools = [...(value.trait_pools || [])]
        pools.splice(data.poolIndex + 1, 0, cloned)
        onChange({ ...value, trait_pools: pools })
        setTimeout(() => handleSelect(`pool:${cloned.id}`), 0)
        break
      }
    }
  }, [value, onChange, handleSelect])

  // ── Delete handler ───────────────────────────────────────────────

  const handleDeleteNode = useCallback((node: TreeNode) => {
    const data = node.data
    if (!data) return

    switch (data.kind) {
      case 'template': {
        const templates = (value.templates || []).filter((_, i) => i !== data.index)
        const initialActors = (value.initial_actors || []).filter((actor) => actor.template_id !== data.template.id)
        onChange({ ...value, templates, initial_actors: initialActors })
        break
      }
      case 'field': {
        const templates = [...(value.templates || [])]
        const tpl = { ...templates[data.templateIndex] }
        tpl.fields = (tpl.fields || []).filter((_, i) => i !== data.fieldIndex)
        templates[data.templateIndex] = tpl
        onChange({ ...value, templates })
        break
      }
      case 'actor': {
        const actors = (value.initial_actors || []).filter((_, i) => i !== data.actorIndex)
        onChange({ ...value, initial_actors: actors })
        break
      }
      case 'pool': {
        const pools = (value.trait_pools || []).filter((_, i) => i !== data.poolIndex)
        const templates = (value.templates || []).map((template) => ({
          ...template,
          trait_rules: (template.trait_rules || []).filter((rule) => rule.pool_id !== data.pool.id),
        }))
        onChange({ ...value, templates, trait_pools: pools })
        break
      }
      case 'trait': {
        const pools = [...(value.trait_pools || [])]
        const pool = { ...pools[data.poolIndex] }
        pool.traits = (pool.traits || []).filter((_, i) => i !== data.traitIndex)
        pools[data.poolIndex] = pool
        const remaining = pool.traits.length
        const templates = (value.templates || []).map((template) => ({
          ...template,
          trait_rules: (template.trait_rules || []).flatMap((rule) => {
            if (rule.pool_id !== pool.id) return [rule]
            return remaining > 0 ? [{ ...rule, draw_count: Math.min(rule.draw_count, remaining) }] : []
          }),
        }))
        onChange({ ...value, templates, trait_pools: pools })
        break
      }
    }
  }, [value, onChange])

  // ── Validity ─────────────────────────────────────────────────────

  const checkValidity = useCallback(() => {
    return isActorStateExplorerValueValid(value)
  }, [value])

  // Report validity changes
  useEffect(() => {
    if (onValidityChange) {
      onValidityChange(checkValidity())
    }
  }, [onValidityChange, checkValidity])

  const attached = layout === 'attached'

  return (
    <div className={cn(
      'actor-state-explorer isolate overflow-hidden',
      attached
        ? 'h-full min-h-0'
        : 'min-h-[320px]',
    )}>
      <div className={cn(
        'grid min-h-0',
        attached
          ? 'actor-state-explorer-layout h-full grid-rows-[minmax(0,1fr)] overflow-hidden'
          : 'min-h-[320px] gap-3 lg:grid-cols-[280px_minmax(0,1fr)]',
      )}>
        {attached ? (
          <button
            type="button"
            className="actor-state-navigation-backdrop"
            data-open={navigatorOpen}
            onClick={() => setNavigatorOpen(false)}
            aria-hidden="true"
            tabIndex={-1}
          />
        ) : null}
        <div className={cn(attached && 'actor-state-navigation h-full min-h-0 overflow-hidden')} data-open={navigatorOpen}>
          <StateTreeNavigator
            attached={attached}
            tree={tree}
            selectedId={selectedId}
            expandedIds={expandedIds}
            onClose={attached ? () => setNavigatorOpen(false) : undefined}
            onSelect={handleSelect}
            onKeyboardSelect={selectAndReveal}
            onToggleExpanded={toggleExpanded}
            onAddTemplate={handleAddTemplate}
            onAddField={handleAddField}
            onAddActor={handleAddActor}
            onAddPool={handleAddPool}
            onAddTrait={handleAddTrait}
          />
        </div>
        <StateDetailArea
          attached={attached}
          tree={tree}
          selectedNode={selectedNode}
          selectedId={selectedId}
          value={value}
          onChange={onChange}
          onNodeIdChange={remapNodeId}
          onOpenNavigation={attached ? () => setNavigatorOpen(true) : undefined}
          onSelect={handleSelect}
          onDuplicateNode={handleDuplicateNode}
          onDeleteNode={handleDeleteNode}
        />
      </div>
    </div>
  )
}
