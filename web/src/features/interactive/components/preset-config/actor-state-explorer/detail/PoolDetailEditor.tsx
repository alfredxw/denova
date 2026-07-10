import { Sparkle, Plus } from 'lucide-react'
import { AnimatePresence, motion } from 'motion/react'
import { useTranslation } from 'react-i18next'
import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'
import { novaEase } from '@/features/motion/motion-tokens'
import { cn } from '@/lib/utils'
import { parseNumberInput, nextPresetId } from '../../utils'
import type { OpeningTrait, OpeningTraitPool } from '../../../../types'
import type { ExplorerProps, TreeNode } from '../types'
import { DetailResponsiveGrid } from './DetailLayout'

interface PoolDetailEditorProps {
  pool: OpeningTraitPool
  poolIndex: number
  selectedId: string
  tree: TreeNode[]
  value: ExplorerProps['value']
  onChange: (value: ExplorerProps['value']) => void
  onSelect: (id: string) => void
}

export function PoolDetailEditor({
  pool,
  poolIndex,
  selectedId,
  value,
  onChange,
  onSelect,
}: PoolDetailEditorProps) {
  const { t } = useTranslation()
  const traits = pool.traits || []

  const updatePool = (patch: Partial<OpeningTraitPool>) => {
    const pools = [...(value.trait_pools || [])]
    pools[poolIndex] = { ...pool, ...patch }
    onChange({ ...value, trait_pools: pools })
  }

  const addTrait = () => {
    const newTrait: OpeningTrait = {
      id: nextPresetId('trait'),
      name: t('settingPanel.actorState.explorer.newTrait', { count: traits.length + 1 }),
      weight: 1,
      ops: [],
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
          <label className="text-[11px] text-[var(--nova-text-faint)]">{t('settingPanel.actorState.explorer.drawCountLabel')}</label>
          <Input
            className="nova-field h-8 text-xs focus-visible:ring-0"
            inputMode="numeric"
            value={pool.draw_count !== undefined ? String(pool.draw_count) : ''}
            onChange={(e) => updatePool({ draw_count: parseNumberInput(e.target.value) ?? 1 })}
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
              const nodeId = `trait:${pool.id || 'pool'}:${trait.id || tIndex}`
              const opCount = (trait.ops || []).length
              return (
                <TraitInlineRow
                  key={trait.id || tIndex}
                  trait={trait}
                  opCount={opCount}
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
  opCount,
  selected,
  onClick,
}: {
  trait: OpeningTrait
  opCount: number
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
        'group flex cursor-pointer items-center gap-2.5 rounded-[12px] border px-3 py-2.5 transition-colors',
        selected
          ? 'border-[var(--nova-accent)]/40 bg-[var(--nova-surface)] shadow-[inset_2px_0_0_var(--nova-accent)]'
          : 'border-[var(--nova-border)] bg-[var(--nova-surface)] hover:border-[var(--nova-accent)]/20 hover:bg-[var(--nova-hover)]',
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
          {opCount > 0 ? (
            <span className="rounded-full border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-1.5 py-0.5 text-[10px] text-[var(--nova-text-faint)]">
              {opCount} ops
            </span>
          ) : null}
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
