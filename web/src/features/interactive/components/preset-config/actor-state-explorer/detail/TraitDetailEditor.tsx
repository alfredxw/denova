import { motion } from 'motion/react'
import { useTranslation } from 'react-i18next'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import { novaEase } from '@/features/motion/motion-tokens'
import { parseNumberInput } from '../../utils'
import type { ActorTraitDefinition, ActorTraitPool } from '../../../../types'
import type { ExplorerProps } from '../types'
import { traitNodeId } from '../build-tree'
import { DetailStack } from './DetailLayout'

interface TraitDetailEditorProps {
  trait: ActorTraitDefinition
  traitIndex: number
  pool: ActorTraitPool
  poolIndex: number
  value: ExplorerProps['value']
  onChange: (value: ExplorerProps['value']) => void
  onIdChange: (nextId: string) => void
}

export function TraitDetailEditor({
  trait,
  traitIndex,
  pool,
  poolIndex,
  value,
  onChange,
  onIdChange,
}: TraitDetailEditorProps) {
  const { t } = useTranslation()
  const updateTrait = (patch: Partial<ActorTraitDefinition>) => {
    const pools = [...(value.trait_pools || [])]
    const p = { ...pools[poolIndex] }
    const traits = [...(p.traits || [])]
    const nextTrait = { ...trait, ...patch }
    traits[traitIndex] = nextTrait
    p.traits = traits
    pools[poolIndex] = p
    const nextNodeId = traitNodeId(pool, nextTrait, traitIndex)
    if (nextNodeId !== traitNodeId(pool, trait, traitIndex)) onIdChange(nextNodeId)
    onChange({ ...value, trait_pools: pools })
  }

  return (
    <DetailStack>
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

        <div className="grid gap-3 sm:grid-cols-2">
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
            <label className="text-[11px] text-[var(--nova-text-faint)]">{t('settingPanel.actorState.explorer.visibility')}</label>
            <Select value={trait.visibility || 'visible'} onValueChange={(visibility) => updateTrait({ visibility: visibility as ActorTraitDefinition['visibility'] })}>
              <SelectTrigger className="nova-field h-8 text-xs focus:ring-0"><SelectValue /></SelectTrigger>
              <SelectContent className="nova-panel border text-[var(--nova-text)]">
                <SelectGroup>
                  {(['visible', 'spoiler', 'hidden'] as const).map((visibility) => (
                    <SelectItem key={visibility} value={visibility}>{t(`settingPanel.actorState.explorer.${visibility}`)}</SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
          </div>
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
    </DetailStack>
  )
}
