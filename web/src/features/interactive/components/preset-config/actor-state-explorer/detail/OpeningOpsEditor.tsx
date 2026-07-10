import { motion } from 'motion/react'
import { useTranslation } from 'react-i18next'
import { novaEase } from '@/features/motion/motion-tokens'
import type { ExplorerProps } from '../types'
import { StateOpBuilder } from '../shared/StateOpBuilder'
import { buildPathSuggestions, buildPathTypeMap, buildPathOptionsMap } from '../path-suggestions'

interface OpeningOpsEditorProps {
  value: ExplorerProps['value']
  onChange: (value: ExplorerProps['value']) => void
}

export function OpeningOpsEditor({ value, onChange }: OpeningOpsEditorProps) {
  const { t } = useTranslation()
  const ops = value.initial_state_ops || []

  const pathSuggestions = buildPathSuggestions(value)
  const pathTypeMap = buildPathTypeMap(value)
  const pathOptionsMap = buildPathOptionsMap(value)

  const handleOpsChange = (newOps: typeof ops) => {
    onChange({ ...value, initial_state_ops: newOps })
  }

  return (
    <div className="flex flex-col gap-4">
      <motion.div
        initial={{ opacity: 0, y: 4 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ duration: 0.2, ease: novaEase }}
      >
        <div className="flex items-center gap-2">
          <span className="text-[11px] font-semibold text-[var(--nova-text-faint)]">
            {t('settingPanel.actorState.explorer.initialOps')}
          </span>
          <span className="rounded-full border border-[var(--nova-border)] bg-[var(--nova-surface)] px-1.5 py-0.5 text-[10px] text-[var(--nova-text-faint)]">
            {ops.length}
          </span>
        </div>
        <p className="mt-1 text-[11px] text-[var(--nova-text-faint)]">
          {t('settingPanel.actorState.explorer.initialOpsDesc')}
        </p>
      </motion.div>

      <StateOpBuilder
        ops={ops}
        onChange={handleOpsChange}
        pathSuggestions={pathSuggestions}
        pathTypeMap={pathTypeMap}
        pathOptionsMap={pathOptionsMap}
      />
    </div>
  )
}
