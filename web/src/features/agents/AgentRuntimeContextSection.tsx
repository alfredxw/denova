import { FolderOpen } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Input } from '@/components/ui/input'
import type { AgentContextOverride } from '@/features/settings/types'
import type { VisibleAgentKey } from './agent-registry'
import { Field, SectionTitle, SwitchWithInheritance } from './agent-form-controls'

export function AgentRuntimeContextSection({ agent, value, inherited, onChange }: {
  agent: VisibleAgentKey
  value: AgentContextOverride
  inherited: AgentContextOverride
  onChange: (patch: Partial<AgentContextOverride>) => void
}) {
  const { t } = useTranslation()
  const hasCompactionEnabled = value.compaction_enabled !== undefined && value.compaction_enabled !== null
  const hasCompactionThreshold = value.compaction_threshold !== undefined && value.compaction_threshold !== null
  const hasCompactionRecentTurns = value.compaction_recent_turns !== undefined && value.compaction_recent_turns !== null
  const hasCompactionTargetMin = value.compaction_target_min_ratio !== undefined && value.compaction_target_min_ratio !== null
  const hasCompactionTargetMax = value.compaction_target_max_ratio !== undefined && value.compaction_target_max_ratio !== null
  const hasToolResultRetention = value.tool_result_retention_enabled !== undefined && value.tool_result_retention_enabled !== null
  const hasMaxFragmentBytes = value.max_fragment_bytes !== undefined && value.max_fragment_bytes !== null
  const hasMaxTotalInjectedBytes = value.max_total_injected_bytes !== undefined && value.max_total_injected_bytes !== null
  const hasMaxFragments = value.max_fragments !== undefined && value.max_fragments !== null
  const hasMaxMetadataFieldBytes = value.max_metadata_field_bytes !== undefined && value.max_metadata_field_bytes !== null
  const hasMaxProviderInputBytes = value.max_provider_input_bytes !== undefined && value.max_provider_input_bytes !== null
  const effectiveCompactionEnabled = hasCompactionEnabled ? value.compaction_enabled : inherited.compaction_enabled ?? true
  const effectiveCompactionThreshold = hasCompactionThreshold ? value.compaction_threshold : inherited.compaction_threshold ?? 0.8
  const effectiveCompactionRecentTurns = hasCompactionRecentTurns ? value.compaction_recent_turns : inherited.compaction_recent_turns ?? 1
  const effectiveCompactionTargetMin = hasCompactionTargetMin ? value.compaction_target_min_ratio : inherited.compaction_target_min_ratio ?? 0.05
  const effectiveCompactionTargetMax = hasCompactionTargetMax ? value.compaction_target_max_ratio : inherited.compaction_target_max_ratio ?? 0.2
  const effectiveToolResultRetention = hasToolResultRetention ? value.tool_result_retention_enabled : inherited.tool_result_retention_enabled ?? (agent === 'ide' || agent === 'interactive_story')
  const effectiveMaxFragmentBytes = hasMaxFragmentBytes ? value.max_fragment_bytes : inherited.max_fragment_bytes ?? 256 * 1024
  const effectiveMaxTotalInjectedBytes = hasMaxTotalInjectedBytes ? value.max_total_injected_bytes : inherited.max_total_injected_bytes ?? 1024 * 1024
  const effectiveMaxFragments = hasMaxFragments ? value.max_fragments : inherited.max_fragments ?? 256
  const effectiveMaxMetadataFieldBytes = hasMaxMetadataFieldBytes ? value.max_metadata_field_bytes : inherited.max_metadata_field_bytes ?? 4 * 1024
  const effectiveMaxProviderInputBytes = hasMaxProviderInputBytes ? value.max_provider_input_bytes : inherited.max_provider_input_bytes ?? 4 * 1024 * 1024
  const isCompactionAgent = agent === 'context_compaction'

  return (
    <section className="flex flex-col gap-3 border-b border-[var(--nova-border)] pb-5">
      <SectionTitle icon={FolderOpen} title={t('agents.section.runtimeContext')} />
      <div className="grid gap-3 md:grid-cols-2">
        {!isCompactionAgent && (
          <>
            <Field label={t('agents.field.compactionEnabled')}>
              <SwitchWithInheritance
                checked={Boolean(effectiveCompactionEnabled)}
                onChange={(checked) => onChange({ compaction_enabled: checked })}
                ariaLabel={t('agents.field.compactionEnabled')}
                inherited={!hasCompactionEnabled}
                onReset={hasCompactionEnabled ? () => onChange({ compaction_enabled: null }) : undefined}
              />
            </Field>
            <Field label={t('agents.field.compactionThreshold')} inherited={!hasCompactionThreshold} onReset={hasCompactionThreshold ? () => onChange({ compaction_threshold: null }) : undefined}>
              <Input
                type="number"
                aria-label={t('agents.field.compactionThreshold')}
                min={50}
                max={98}
                step={1}
                value={Math.round((effectiveCompactionThreshold ?? 0.8) * 100)}
                onChange={(e) => onChange({ compaction_threshold: e.target.value === '' ? null : Number(e.target.value) / 100 })}
                className="h-7 flex-1 text-xs"
              />
            </Field>
            <Field label={t('agents.field.toolResultRetention')}>
              <SwitchWithInheritance
                checked={Boolean(effectiveToolResultRetention)}
                onChange={(checked) => onChange({ tool_result_retention_enabled: checked })}
                ariaLabel={t('agents.field.toolResultRetention')}
                inherited={!hasToolResultRetention}
                onReset={hasToolResultRetention ? () => onChange({ tool_result_retention_enabled: null }) : undefined}
              />
            </Field>
          </>
        )}
        {isCompactionAgent && (
          <>
            <Field label={t('agents.field.compactionRecentTurns')} inherited={!hasCompactionRecentTurns} onReset={hasCompactionRecentTurns ? () => onChange({ compaction_recent_turns: null }) : undefined}>
              <Input
                type="number"
                aria-label={t('agents.field.compactionRecentTurns')}
                min={1}
                max={30}
                step={1}
                value={effectiveCompactionRecentTurns ?? 1}
                onChange={(e) => onChange({ compaction_recent_turns: e.target.value === '' ? null : Number(e.target.value) })}
                className="h-7 flex-1 text-xs"
              />
            </Field>
            <Field label={t('agents.field.compactionTargetMin')} inherited={!hasCompactionTargetMin} onReset={hasCompactionTargetMin ? () => onChange({ compaction_target_min_ratio: null }) : undefined}>
              <Input
                type="number"
                aria-label={t('agents.field.compactionTargetMin')}
                min={1}
                max={80}
                step={1}
                value={Math.round((effectiveCompactionTargetMin ?? 0.05) * 100)}
                onChange={(e) => onChange({ compaction_target_min_ratio: e.target.value === '' ? null : Number(e.target.value) / 100 })}
                className="h-7 flex-1 text-xs"
              />
            </Field>
            <Field label={t('agents.field.compactionTargetMax')} inherited={!hasCompactionTargetMax} onReset={hasCompactionTargetMax ? () => onChange({ compaction_target_max_ratio: null }) : undefined}>
              <Input
                type="number"
                aria-label={t('agents.field.compactionTargetMax')}
                min={1}
                max={80}
                step={1}
                value={Math.round((effectiveCompactionTargetMax ?? 0.2) * 100)}
                onChange={(e) => onChange({ compaction_target_max_ratio: e.target.value === '' ? null : Number(e.target.value) / 100 })}
                className="h-7 flex-1 text-xs"
              />
            </Field>
          </>
        )}
        <Field label={t('agents.field.maxFragmentKB')} inherited={!hasMaxFragmentBytes} onReset={hasMaxFragmentBytes ? () => onChange({ max_fragment_bytes: null }) : undefined}>
          <Input
            type="number"
            aria-label={t('agents.field.maxFragmentKB')}
            min={1}
            max={16384}
            step={1}
            value={Math.round((effectiveMaxFragmentBytes ?? 256 * 1024) / 1024)}
            onChange={(e) => onChange({ max_fragment_bytes: e.target.value === '' ? null : Number(e.target.value) * 1024 })}
            className="h-7 flex-1 text-xs"
          />
        </Field>
        <Field label={t('agents.field.maxInjectedKB')} inherited={!hasMaxTotalInjectedBytes} onReset={hasMaxTotalInjectedBytes ? () => onChange({ max_total_injected_bytes: null }) : undefined}>
          <Input
            type="number"
            aria-label={t('agents.field.maxInjectedKB')}
            min={1}
            max={65536}
            step={1}
            value={Math.round((effectiveMaxTotalInjectedBytes ?? 1024 * 1024) / 1024)}
            onChange={(e) => onChange({ max_total_injected_bytes: e.target.value === '' ? null : Number(e.target.value) * 1024 })}
            className="h-7 flex-1 text-xs"
          />
        </Field>
        <Field label={t('agents.field.maxContextFragments')} inherited={!hasMaxFragments} onReset={hasMaxFragments ? () => onChange({ max_fragments: null }) : undefined}>
          <Input
            type="number"
            aria-label={t('agents.field.maxContextFragments')}
            min={1}
            max={4096}
            step={1}
            value={effectiveMaxFragments ?? 256}
            onChange={(e) => onChange({ max_fragments: e.target.value === '' ? null : Number(e.target.value) })}
            className="h-7 flex-1 text-xs"
          />
        </Field>
        <Field label={t('agents.field.maxContextMetadataKB')} inherited={!hasMaxMetadataFieldBytes} onReset={hasMaxMetadataFieldBytes ? () => onChange({ max_metadata_field_bytes: null }) : undefined}>
          <Input
            type="number"
            aria-label={t('agents.field.maxContextMetadataKB')}
            min={1}
            max={64}
            step={1}
            value={Math.round((effectiveMaxMetadataFieldBytes ?? 4 * 1024) / 1024)}
            onChange={(e) => onChange({ max_metadata_field_bytes: e.target.value === '' ? null : Number(e.target.value) * 1024 })}
            className="h-7 flex-1 text-xs"
          />
        </Field>
        <Field label={t('agents.field.maxProviderInputKB')} inherited={!hasMaxProviderInputBytes} onReset={hasMaxProviderInputBytes ? () => onChange({ max_provider_input_bytes: null }) : undefined}>
          <Input
            type="number"
            aria-label={t('agents.field.maxProviderInputKB')}
            min={129}
            max={65536}
            step={128}
            value={Math.round((effectiveMaxProviderInputBytes ?? 4 * 1024 * 1024) / 1024)}
            onChange={(e) => onChange({ max_provider_input_bytes: e.target.value === '' ? null : Number(e.target.value) * 1024 })}
            className="h-7 flex-1 text-xs"
          />
        </Field>
      </div>
      <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-2 text-[11px] leading-5 text-[var(--nova-text-faint)]">
        {isCompactionAgent ? t('agents.context.compactionTargetNote') : t('agents.context.compactionNote')}
        {!isCompactionAgent && <div className="mt-1">{t('agents.context.toolResultRetentionNote')}</div>}
        <div className="mt-1">{t('agents.context.assemblyBudgetNote')}</div>
      </div>
    </section>
  )
}
