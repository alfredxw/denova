import { Hash, Layers, Plus } from 'lucide-react'
import { AnimatePresence, motion } from 'motion/react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { cn } from '@/lib/utils'
import { novaEase } from '@/features/motion/motion-tokens'
import type { ActorStateField, ActorStateTemplate } from '../../../../types'
import type { ExplorerProps, TreeNode } from '../types'
import { fieldNodeId, findNode, templateNodeId } from '../build-tree'
import { FieldTypeBadge } from '../shared/FieldTypeBadge'
import { DetailResponsiveGrid, DetailStack } from './DetailLayout'

interface TemplateDetailEditorProps {
  template: ActorStateTemplate
  templateIndex: number
  selectedId: string
  tree: TreeNode[]
  value: ExplorerProps['value']
  onChange: (value: ExplorerProps['value']) => void
  onIdChange: (nextId: string) => void
  onSelect: (id: string) => void
}

export function TemplateDetailEditor({
  template,
  templateIndex,
  selectedId,
  tree,
  value,
  onChange,
  onIdChange,
  onSelect,
}: TemplateDetailEditorProps) {
  const { t } = useTranslation()
  const fields = template.fields || []
  const traitRules = template.trait_rules || []
  const traitPools = value.trait_pools || []

  const updateTemplate = (patch: Partial<ActorStateTemplate>) => {
    const templates = [...(value.templates || [])]
    const nextTemplate = { ...template, ...patch }
    templates[templateIndex] = nextTemplate
    if (patch.id === undefined || patch.id === template.id) {
      onChange({ ...value, templates })
      return
    }

    const initialActors = (value.initial_actors || []).map((actor) => (
      actor.template_id === template.id
        ? { ...actor, template_id: nextTemplate.id }
        : actor
    ))
    onIdChange(templateNodeId(nextTemplate, templateIndex))
    onChange({ ...value, templates, initial_actors: initialActors })
  }

  const addField = () => {
    const newField: ActorStateField = {
      name: t('settingPanel.actorState.explorer.newField', { count: fields.length + 1 }),
      type: 'string',
    }
    updateTemplate({ fields: [...fields, newField] })
  }

  const toggleTraitPool = (poolId: string, enabled: boolean) => {
    const nextRules = enabled
      ? [...traitRules, { pool_id: poolId, draw_count: 1 }]
      : traitRules.filter((rule) => rule.pool_id !== poolId)
    updateTemplate({ trait_rules: nextRules })
  }

  const updateTraitRuleCount = (poolId: string, drawCount: number) => {
    updateTemplate({
      trait_rules: traitRules.map((rule) => (
        rule.pool_id === poolId
          ? { ...rule, draw_count: Math.max(1, Math.floor(drawCount || 1)) }
          : rule
      )),
    })
  }

  return (
    <DetailStack>
      {/* Template basic info */}
      <motion.section
        initial={{ opacity: 0, y: 4 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ duration: 0.2, ease: novaEase }}
        className="space-y-3"
      >
        <div className="flex items-center gap-2">
          <span className="text-[11px] font-semibold text-[var(--nova-text-faint)]">
            {t('settingPanel.actorState.explorer.templateInfo')}
          </span>
        </div>
        <div className="grid gap-3 sm:grid-cols-2">
          <div className="space-y-1">
            <label className="text-[11px] text-[var(--nova-text-faint)]">ID</label>
            <Input
              className="nova-field h-8 text-xs focus-visible:ring-0"
              value={template.id}
              onChange={(e) => updateTemplate({ id: e.target.value })}
            />
          </div>
          <div className="space-y-1">
            <label className="text-[11px] text-[var(--nova-text-faint)]">{t('settingPanel.field.name')}</label>
            <Input
              className="nova-field h-8 text-xs focus-visible:ring-0"
              value={template.name || ''}
              onChange={(e) => updateTemplate({ name: e.target.value })}
              placeholder={t('settingPanel.actorState.explorer.templateNamePlaceholder')}
            />
          </div>
        </div>
        <div className="space-y-1">
          <label className="text-[11px] text-[var(--nova-text-faint)]">{t('common.description')}</label>
          <Textarea
            className="nova-field min-h-[60px] resize-none text-xs focus-visible:ring-0"
            value={template.description || ''}
            onChange={(e) => updateTemplate({ description: e.target.value })}
            placeholder={t('settingPanel.actorState.explorer.templateDescriptionPlaceholder')}
          />
        </div>
      </motion.section>

      <DetailResponsiveGrid className="items-start">
        <section className="space-y-2">
          {/* Fields section */}
          <SectionHeader
            title={t('settingPanel.actorState.fields')}
            count={fields.length}
            onAdd={addField}
            addLabel={t('settingPanel.actorState.addField')}
          />
          <div className="space-y-1.5">
            <AnimatePresence initial={false}>
              {fields.map((field, fIndex) => {
                const nodeId = fieldNodeId(template.id, field, fIndex)
                const node = findNode(tree, nodeId)
                const isSelected = node ? nodeId === selectedId : false
                return (
                  <FieldInlineRow
					key={fIndex}
                    field={field}
                    selected={isSelected}
                    onClick={() => onSelect(nodeId)}
                  />
                )
              })}
            </AnimatePresence>
            {fields.length === 0 ? (
              <EmptyHint text={t('settingPanel.actorState.explorer.emptyTemplateFields')} />
            ) : null}
          </div>
        </section>

        <div className="space-y-6">
        <section className="space-y-2">
          <SectionHeader
            title={t('settingPanel.actorState.explorer.templateTraitRules')}
            count={traitRules.length}
          />
          <div className="space-y-1.5">
            {traitPools.map((pool) => {
              const rule = traitRules.find((candidate) => candidate.pool_id === pool.id)
              const switchLabel = t('settingPanel.actorState.explorer.togglePoolForTemplate', {
                pool: pool.name || pool.id,
              })
              return (
                <motion.div
                  layout
                  key={pool.id}
                  className={cn(
                    'rounded-[12px] border px-3 py-2.5 transition-colors',
                    rule
                      ? 'border-[var(--nova-accent)]/30 bg-[var(--nova-surface)]'
                      : 'border-[var(--nova-border)] bg-[var(--nova-surface)]',
                  )}
                >
                  <div className="flex items-center gap-2.5">
                    <Layers className={cn(
                      'h-3.5 w-3.5 shrink-0',
                      rule ? 'text-[var(--nova-accent)]' : 'text-[var(--nova-text-faint)]',
                    )} />
                    <div className="min-w-0 flex-1">
                      <div className="truncate text-[12px] font-medium text-[var(--nova-text)]">
                        {pool.name || pool.id}
                      </div>
                      <div className="truncate font-mono text-[10px] text-[var(--nova-text-faint)]">
                        {pool.id} · {t('settingPanel.actorState.explorer.traitCount', { count: pool.traits?.length || 0 })}
                      </div>
                    </div>
                    <Switch
                      checked={Boolean(rule)}
                      onCheckedChange={(checked) => toggleTraitPool(pool.id, checked)}
                      aria-label={switchLabel}
                      title={switchLabel}
                    />
                  </div>
                  {rule ? (
                    <div className="mt-2.5 flex items-center justify-between gap-3 border-t border-[var(--nova-border)] pt-2.5">
                      <label className="text-[11px] text-[var(--nova-text-faint)]" htmlFor={`trait-count-${template.id}-${pool.id}`}>
                        {t('settingPanel.actorState.explorer.drawCountForPool')}
                      </label>
                      <Input
                        id={`trait-count-${template.id}-${pool.id}`}
                        type="number"
                        min={1}
                        max={Math.max(1, pool.traits?.length || 1)}
                        className="nova-field h-7 w-20 text-xs focus-visible:ring-0"
                        value={rule.draw_count}
                        onChange={(event) => updateTraitRuleCount(pool.id, Number(event.target.value))}
                      />
                    </div>
                  ) : null}
                </motion.div>
              )
            })}
            {traitPools.length === 0 ? (
              <EmptyHint text={t('settingPanel.actorState.explorer.emptyTraitLibrary')} />
            ) : null}
          </div>
        </section>
        </div>
      </DetailResponsiveGrid>
    </DetailStack>
  )
}

// ── Sub-components ────────────────────────────────────────────────

function SectionHeader({
  title,
  count,
  onAdd,
  addLabel,
}: {
  title: string
  count: number
  onAdd?: () => void
  addLabel?: string
}) {
  return (
    <div className="flex items-center justify-between">
      <div className="flex items-center gap-2">
        <span className="text-[11px] font-semibold text-[var(--nova-text-faint)]">
          {title}
        </span>
        <span className="rounded-full border border-[var(--nova-border)] bg-[var(--nova-surface)] px-1.5 py-0.5 text-[10px] text-[var(--nova-text-faint)]">
          {count}
        </span>
      </div>
      {onAdd && addLabel ? (
        <Button
          type="button"
          variant="ghost"
          size="sm"
          className="h-7 rounded-full px-2.5 text-[11px] text-[var(--nova-text-faint)] hover:text-[var(--nova-text)]"
          onClick={onAdd}
        >
          <Plus className="mr-1 h-3.5 w-3.5" />
          {addLabel}
        </Button>
      ) : null}
    </div>
  )
}

function FieldInlineRow({
  field,
  selected,
  onClick,
}: {
  field: ActorStateField
  selected: boolean
  onClick: () => void
}) {
  const { t } = useTranslation()
  return (
    <motion.div
      layout
      initial={{ opacity: 0, y: -4 }}
      animate={{ opacity: 1, y: 0 }}
      exit={{ opacity: 0, y: -4 }}
      transition={{ duration: 0.15, ease: novaEase }}
      className={cn(
        'group flex min-h-10 cursor-pointer items-center gap-2 rounded-[9px] border px-2.5 py-1.5 transition-colors',
        selected
          ? 'border-[var(--nova-border)] bg-[var(--nova-active)]'
          : 'border-transparent bg-transparent hover:bg-[var(--nova-hover)]',
      )}
      onClick={onClick}
    >
      <Hash className={cn(
        'h-3.5 w-3.5 shrink-0',
        selected ? 'text-[var(--nova-accent)]' : 'text-[var(--nova-text-faint)]',
      )} />
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-center gap-1.5">
          <span className="truncate text-[12px] font-medium text-[var(--nova-text)]">
			{field.name || t('settingPanel.actorState.explorer.unnamedField')}
          </span>
          <FieldTypeBadge type={field.type} />
        </div>
		<div className="mt-0.5 truncate text-[10px] text-[var(--nova-text-faint)]">{t('settingPanel.actorState.explorer.nameIsId')}</div>
      </div>
    </motion.div>
  )
}

function EmptyHint({ text }: { text: string }) {
  return (
    <div className="rounded-[12px] border border-dashed border-[var(--nova-border)] bg-[var(--nova-surface)] px-4 py-6 text-center text-[11px] text-[var(--nova-text-faint)]">
      {text}
    </div>
  )
}
