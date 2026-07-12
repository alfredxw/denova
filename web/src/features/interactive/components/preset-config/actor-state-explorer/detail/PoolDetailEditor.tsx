import { Sparkle, Plus } from 'lucide-react'
import { AnimatePresence, motion } from 'motion/react'
import { useTranslation } from 'react-i18next'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Button } from '@/components/ui/button'
import { novaEase } from '@/features/motion/motion-tokens'
import { cn } from '@/lib/utils'
import { nextPresetId } from '../../utils'
import type { ActorTraitDefinition, ActorTraitPool } from '../../../../types'
import type { ExplorerProps, TreeNode } from '../types'
import { poolNodeId, traitNodeId } from '../build-tree'
import { DetailResponsiveGrid } from './DetailLayout'

interface PoolDetailEditorProps {
  pool: ActorTraitPool
  poolIndex: number
  selectedId: string
  tree: TreeNode[]
  value: ExplorerProps['value']
  onChange: (value: ExplorerProps['value']) => void
  onIdChange: (nextId: string) => void
  onSelect: (id: string) => void
}

export function PoolDetailEditor({
  pool,
  poolIndex,
  selectedId,
  value,
  onChange,
  onIdChange,
  onSelect,
}: PoolDetailEditorProps) {
  const { t } = useTranslation()
  const traits = pool.traits || []

  const updatePool = (patch: Partial<ActorTraitPool>) => {
    const pools = [...(value.trait_pools || [])]
    const nextPool = { ...pool, ...patch }
    pools[poolIndex] = nextPool
    const templates = patch.id !== undefined && patch.id !== pool.id
      ? (value.templates || []).map((template) => ({
          ...template,
          trait_rules: (template.trait_rules || []).map((rule) => rule.pool_id === pool.id ? { ...rule, pool_id: nextPool.id } : rule),
        }))
      : value.templates
    const nextNodeId = poolNodeId(nextPool, poolIndex)
    if (nextNodeId !== poolNodeId(pool, poolIndex)) onIdChange(nextNodeId)
    onChange({ ...value, templates, trait_pools: pools })
  }

  const addTrait = () => {
    const newTrait: ActorTraitDefinition = {
      id: nextPresetId('trait'),
      name: t('settingPanel.actorState.explorer.newTrait', { count: traits.length + 1 }),
      weight: 1,
      visibility: 'visible',
    }
    updatePool({ traits: [...traits, newTrait] })
  }

  return (
    <DetailResponsiveGrid className="items-start">
      {/* Pool info */}
      <motion.section
        initial={{ opacity: 0, y: 4 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ duration: 0.2, ease: novaEase }}
        className="space-y-3"
      >
        <div className="flex items-center gap-2">
          <span className="text-[11px] font-semibold text-[var(--nova-text-faint)]">
            {t('settingPanel.actorState.explorer.poolInfo')}
          </span>
        </div>

        <div className="grid gap-3 sm:grid-cols-2">
          <div className="space-y-1">
            <label className="text-[11px] text-[var(--nova-text-faint)]">ID</label>
            <Input
              className="nova-field h-8 text-xs focus-visible:ring-0"
              value={pool.id || ''}
              onChange={(e) => updatePool({ id: e.target.value })}
            />
          </div>
          <div className="space-y-1">
            <label className="text-[11px] text-[var(--nova-text-faint)]">{t('settingPanel.field.name')}</label>
            <Input
              className="nova-field h-8 text-xs focus-visible:ring-0"
              value={pool.name || ''}
              onChange={(e) => updatePool({ name: e.target.value })}
              placeholder={t('settingPanel.actorState.explorer.poolNamePlaceholder')}
            />
          </div>
        </div>

        <div className="space-y-1">
          <label className="text-[11px] text-[var(--nova-text-faint)]">{t('common.description')}</label>
          <Textarea
            className="nova-field min-h-[64px] resize-none text-xs focus-visible:ring-0"
            value={pool.description || ''}
            onChange={(e) => updatePool({ description: e.target.value })}
            placeholder={t('settingPanel.actorState.explorer.poolDescriptionPlaceholder')}
          />
        </div>
      </motion.section>

      {/* Traits list */}
      <div>
        <div className="mb-2 flex items-center justify-between">
          <div className="flex items-center gap-2">
            <span className="text-[11px] font-semibold text-[var(--nova-text-faint)]">
              {t('settingPanel.actorState.explorer.traits')}
            </span>
            <span className="rounded-full border border-[var(--nova-border)] bg-[var(--nova-surface)] px-1.5 py-0.5 text-[10px] text-[var(--nova-text-faint)]">
              {traits.length}
            </span>
          </div>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            className="h-7 rounded-full px-2.5 text-[11px] text-[var(--nova-text-faint)] hover:text-[var(--nova-text)]"
            onClick={addTrait}
          >
            <Plus data-icon="inline-start" />
            {t('settingPanel.actorState.explorer.addTrait')}
          </Button>
        </div>

        <div className="space-y-1.5">
          <AnimatePresence initial={false}>
            {traits.map((trait, tIndex) => {
              const nodeId = traitNodeId(pool, trait, tIndex)
              return (
                <TraitInlineRow
                  key={trait.id || tIndex}
                  trait={trait}
                  selected={nodeId === selectedId}
                  onClick={() => onSelect(nodeId)}
                />
              )
            })}
          </AnimatePresence>
          {traits.length === 0 ? (
            <div className="rounded-[12px] border border-dashed border-[var(--nova-border)] bg-[var(--nova-surface)] px-4 py-6 text-center text-[11px] text-[var(--nova-text-faint)]">
              {t('settingPanel.actorState.explorer.emptyPool')}
            </div>
          ) : null}
        </div>
      </div>
    </DetailResponsiveGrid>
  )
}

function TraitInlineRow({
  trait,
  selected,
  onClick,
}: {
  trait: ActorTraitDefinition
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
      <Sparkle className={cn(
        'h-3.5 w-3.5 shrink-0',
        selected ? 'text-[var(--nova-accent)]' : 'text-[var(--nova-text-faint)]',
      )} />
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-1.5">
          <span className="truncate text-[12px] font-medium text-[var(--nova-text)]">
            {trait.name || trait.id || t('settingPanel.actorState.explorer.unnamedTrait')}
          </span>
          <span className="rounded-full border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-1.5 py-0.5 text-[10px] text-[var(--nova-text-faint)]">
            {t('settingPanel.actorState.explorer.weight', { count: trait.weight ?? 1 })}
          </span>
        </div>
        {trait.summary ? (
          <div className="mt-0.5 truncate text-[10px] text-[var(--nova-text-faint)]">
            {trait.summary}
          </div>
        ) : null}
      </div>
    </motion.div>
  )
}
