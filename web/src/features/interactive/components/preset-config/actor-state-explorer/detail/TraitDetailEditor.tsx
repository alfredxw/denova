import { motion } from 'motion/react'
import { useTranslation } from 'react-i18next'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { novaEase } from '@/features/motion/motion-tokens'
import { parseNumberInput } from '../../utils'
import type { OpeningTrait, OpeningTraitPool } from '../../../../types'
import type { ExplorerProps } from '../types'
import { StateOpBuilder } from '../shared/StateOpBuilder'
import { buildPathSuggestions, buildPathTypeMap, buildPathOptionsMap } from '../path-suggestions'
import { DetailResponsiveGrid } from './DetailLayout'

interface TraitDetailEditorProps {
  trait: OpeningTrait
  traitIndex: number
  pool: OpeningTraitPool
  poolIndex: number
  value: ExplorerProps['value']
  onChange: (value: ExplorerProps['value']) => void
}

export function TraitDetailEditor({
  trait,
  traitIndex,
  pool,
  poolIndex,
  value,
  onChange,
}: TraitDetailEditorProps) {
  const { t } = useTranslation()
  const ops = trait.ops || []

  const pathSuggestions = buildPathSuggestions(value)
  const pathTypeMap = buildPathTypeMap(value)
  const pathOptionsMap = buildPathOptionsMap(value)

  const updateTrait = (patch: Partial<OpeningTrait>) => {
    const pools = [...(value.trait_pools || [])]
    const p = { ...pools[poolIndex] }
    const traits = [...(p.traits || [])]
    traits[traitIndex] = { ...trait, ...patch }
    p.traits = traits
    pools[poolIndex] = p
    onChange({ ...value, trait_pools: pools })
  }

  const handleOpsChange = (newOps: typeof ops) => {
    updateTrait({ ops: newOps })
  }

  return (
    <DetailResponsiveGrid className="items-start" minWidth={340}>
      {/* Trait info */}
      <motion.section
        initial={{ opacity: 0, y: 4 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ duration: 0.2, ease: novaEase }}
        className="space-y-3"
      >
        <div className="flex items-center gap-2">
          <span className="text-[11px] font-semibold text-[var(--nova-text-faint)]">
            {t('settingPanel.actorState.explorer.traitInfo')}
          </span>
          <span className="rounded-full border border-[var(--nova-border)] bg-[var(--nova-surface)] px-1.5 py-0.5 text-[10px] text-[var(--nova-text-faint)]">
            {pool.name || pool.id}
          </span>
        </div>

        <div className="grid gap-3 sm:grid-cols-2">
          <div className="space-y-1">
            <label className="text-[11px] text-[var(--nova-text-faint)]">ID</label>
            <Input
              className="nova-field h-8 text-xs focus-visible:ring-0"
              value={trait.id || ''}
              onChange={(e) => updateTrait({ id: e.target.value })}
            />
          </div>
          <div className="space-y-1">
            <label className="text-[11px] text-[var(--nova-text-faint)]">{t('settingPanel.field.name')}</label>
            <Input
              className="nova-field h-8 text-xs focus-visible:ring-0"
              value={trait.name || ''}
              onChange={(e) => updateTrait({ name: e.target.value })}
              placeholder={t('settingPanel.actorState.explorer.traitNamePlaceholder')}
            />
          </div>
        </div>

        <div className="space-y-1">
          <label className="text-[11px] text-[var(--nova-text-faint)]">{t('settingPanel.actorState.explorer.weightLabel')}</label>
          <Input
            className="nova-field h-8 text-xs focus-visible:ring-0"
            inputMode="decimal"
            value={trait.weight !== undefined ? String(trait.weight) : ''}
            onChange={(e) => updateTrait({ weight: parseNumberInput(e.target.value) ?? 1 })}
          />
        </div>

        <div className="space-y-1">
          <label className="text-[11px] text-[var(--nova-text-faint)]">{t('settingPanel.actorState.explorer.summary')}</label>
          <Textarea
            className="nova-field min-h-[48px] resize-none text-xs focus-visible:ring-0"
            value={trait.summary || ''}
            onChange={(e) => updateTrait({ summary: e.target.value })}
            placeholder={t('settingPanel.actorState.explorer.traitDescriptionPlaceholder')}
          />
        </div>
      </motion.section>

      {/* State operations */}
      <section className="space-y-2">
        <div className="flex items-center gap-2">
          <span className="text-[11px] font-semibold text-[var(--nova-text-faint)]">
            {t('settingPanel.actorState.explorer.stateOps')}
          </span>
          <span className="rounded-full border border-[var(--nova-border)] bg-[var(--nova-surface)] px-1.5 py-0.5 text-[10px] text-[var(--nova-text-faint)]">
            {ops.length}
          </span>
        </div>
        <StateOpBuilder
          ops={ops}
          onChange={handleOpsChange}
          pathSuggestions={pathSuggestions}
          pathTypeMap={pathTypeMap}
          pathOptionsMap={pathOptionsMap}
        />
      </section>
    </DetailResponsiveGrid>
  )
}
