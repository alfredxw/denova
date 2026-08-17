import { FolderOpen } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Input } from '@/components/ui/input'
import type { AgentContextOverride, ResolvedAgentContextSettings } from '@/features/settings/types'
import { Field, SectionTitle, SwitchWithInheritance } from './agent-form-controls'

export function AgentRuntimeContextSection({ value, resolved, onChange }: {
  value: AgentContextOverride
  resolved: ResolvedAgentContextSettings
  onChange: (patch: Partial<AgentContextOverride>) => void
}) {
  const { t } = useTranslation()
  const hasCompactionEnabled = value.compaction_enabled != null
  const hasCompactionThreshold = value.compaction_threshold != null
  const hasToolResultContext = value.tool_result_context_enabled != null
  const hasMaxFragmentBytes = value.max_fragment_bytes != null
  const hasMaxTotalInjectedBytes = value.max_total_injected_bytes != null
  const hasMaxFragments = value.max_fragments != null
  const hasMaxMetadataFieldBytes = value.max_metadata_field_bytes != null
  const hasMaxProviderInputBytes = value.max_provider_input_bytes != null
  const compactionEnabled = value.compaction_enabled ?? resolved.compaction_enabled
  const compactionThreshold = value.compaction_threshold ?? resolved.compaction_threshold
  const toolResultContextEnabled = value.tool_result_context_enabled ?? resolved.tool_result_context_enabled
  const maxFragmentBytes = value.max_fragment_bytes ?? resolved.max_fragment_bytes
  const maxTotalInjectedBytes = value.max_total_injected_bytes ?? resolved.max_total_injected_bytes
  const maxFragments = value.max_fragments ?? resolved.max_fragments
  const maxMetadataFieldBytes = value.max_metadata_field_bytes ?? resolved.max_metadata_field_bytes
  const maxProviderInputBytes = value.max_provider_input_bytes ?? resolved.max_provider_input_bytes

  return (
    <section className="flex flex-col gap-3 border-b border-[var(--nova-border)] pb-5">
      <SectionTitle icon={FolderOpen} title={t('agents.section.runtimeContext')} />
      <div className="grid gap-3 md:grid-cols-2">
        <Field label={t('agents.field.compactionEnabled')}>
          <SwitchWithInheritance
            checked={compactionEnabled}
            onChange={(checked) => onChange({ compaction_enabled: checked })}
            ariaLabel={t('agents.field.compactionEnabled')}
            inherited={!hasCompactionEnabled}
            onReset={hasCompactionEnabled ? () => onChange({ compaction_enabled: null }) : undefined}
          />
        </Field>
        <Field
          label={t('agents.field.compactionThreshold')}
          inherited={!hasCompactionThreshold}
          onReset={hasCompactionThreshold ? () => onChange({ compaction_threshold: null }) : undefined}
        >
          <Input
            type="number"
            aria-label={t('agents.field.compactionThreshold')}
            min={50}
            max={98}
            step={1}
            value={Math.round(compactionThreshold * 100)}
            onChange={(event) => onChange({
              compaction_threshold: event.target.value === '' ? null : Number(event.target.value) / 100,
            })}
            className="h-7 flex-1 text-xs"
          />
        </Field>
        <Field label={t('agents.field.toolResultContext')}>
          <SwitchWithInheritance
            checked={toolResultContextEnabled}
            onChange={(checked) => onChange({ tool_result_context_enabled: checked })}
            ariaLabel={t('agents.field.toolResultContext')}
            inherited={!hasToolResultContext}
            onReset={hasToolResultContext ? () => onChange({ tool_result_context_enabled: null }) : undefined}
          />
        </Field>
        <Field
          label={t('agents.field.maxFragmentKB')}
          inherited={!hasMaxFragmentBytes}
          onReset={hasMaxFragmentBytes ? () => onChange({ max_fragment_bytes: null }) : undefined}
        >
          <Input
            type="number"
            aria-label={t('agents.field.maxFragmentKB')}
            min={1}
            max={16384}
            step={1}
            value={Math.round(maxFragmentBytes / 1024)}
            onChange={(event) => onChange({ max_fragment_bytes: event.target.value === '' ? null : Number(event.target.value) * 1024 })}
            className="h-7 flex-1 text-xs"
          />
        </Field>
        <Field
          label={t('agents.field.maxInjectedKB')}
          inherited={!hasMaxTotalInjectedBytes}
          onReset={hasMaxTotalInjectedBytes ? () => onChange({ max_total_injected_bytes: null }) : undefined}
        >
          <Input
            type="number"
            aria-label={t('agents.field.maxInjectedKB')}
            min={1}
            max={65536}
            step={1}
            value={Math.round(maxTotalInjectedBytes / 1024)}
            onChange={(event) => onChange({ max_total_injected_bytes: event.target.value === '' ? null : Number(event.target.value) * 1024 })}
            className="h-7 flex-1 text-xs"
          />
        </Field>
        <Field
          label={t('agents.field.maxContextFragments')}
          inherited={!hasMaxFragments}
          onReset={hasMaxFragments ? () => onChange({ max_fragments: null }) : undefined}
        >
          <Input
            type="number"
            aria-label={t('agents.field.maxContextFragments')}
            min={1}
            max={4096}
            step={1}
            value={maxFragments}
            onChange={(event) => onChange({ max_fragments: event.target.value === '' ? null : Number(event.target.value) })}
            className="h-7 flex-1 text-xs"
          />
        </Field>
        <Field
          label={t('agents.field.maxContextMetadataKB')}
          inherited={!hasMaxMetadataFieldBytes}
          onReset={hasMaxMetadataFieldBytes ? () => onChange({ max_metadata_field_bytes: null }) : undefined}
        >
          <Input
            type="number"
            aria-label={t('agents.field.maxContextMetadataKB')}
            min={1}
            max={64}
            step={1}
            value={Math.round(maxMetadataFieldBytes / 1024)}
            onChange={(event) => onChange({ max_metadata_field_bytes: event.target.value === '' ? null : Number(event.target.value) * 1024 })}
            className="h-7 flex-1 text-xs"
          />
        </Field>
        <Field
          label={t('agents.field.maxProviderInputKB')}
          inherited={!hasMaxProviderInputBytes}
          onReset={hasMaxProviderInputBytes ? () => onChange({ max_provider_input_bytes: null }) : undefined}
        >
          <Input
            type="number"
            aria-label={t('agents.field.maxProviderInputKB')}
            min={129}
            max={65536}
            step={128}
            value={Math.round(maxProviderInputBytes / 1024)}
            onChange={(event) => onChange({ max_provider_input_bytes: event.target.value === '' ? null : Number(event.target.value) * 1024 })}
            className="h-7 flex-1 text-xs"
          />
        </Field>
      </div>
      <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-2 text-[11px] leading-5 text-[var(--nova-text-faint)]">
        {t('agents.context.compactionNote')}
        <div className="mt-1">{t('agents.context.toolResultContextNote')}</div>
        <div className="mt-1">{t('agents.context.assemblyBudgetNote')}</div>
      </div>
    </section>
  )
}
