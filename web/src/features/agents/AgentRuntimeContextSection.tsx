import { FolderOpen } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
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
  const hasContextPressureScope = value.context_pressure_scope !== undefined && value.context_pressure_scope !== null
  const hasToolResultCleanupThreshold = value.tool_result_cleanup_threshold !== undefined && value.tool_result_cleanup_threshold !== null
  const hasToolResultCleanupTarget = value.tool_result_cleanup_target !== undefined && value.tool_result_cleanup_target !== null
  const hasToolResultCleanupMinTokens = value.tool_result_cleanup_min_tokens !== undefined && value.tool_result_cleanup_min_tokens !== null
  const hasToolResultKeepRecent = value.tool_result_keep_recent !== undefined && value.tool_result_keep_recent !== null
  const hasToolResultKeepRecentTokens = value.tool_result_keep_recent_tokens !== undefined && value.tool_result_keep_recent_tokens !== null
  const hasToolResultWarmSuffixTokens = value.tool_result_warm_suffix_tokens !== undefined && value.tool_result_warm_suffix_tokens !== null
  const hasToolResultEagerMinTokens = value.tool_result_eager_min_tokens !== undefined && value.tool_result_eager_min_tokens !== null
  const hasCompactionRecentTurns = value.compaction_recent_turns !== undefined && value.compaction_recent_turns !== null
  const hasCompactionTargetMin = value.compaction_target_min_ratio !== undefined && value.compaction_target_min_ratio !== null
  const hasCompactionTargetMax = value.compaction_target_max_ratio !== undefined && value.compaction_target_max_ratio !== null
  const hasCompactionRecoveryBand = value.compaction_recovery_band !== undefined && value.compaction_recovery_band !== null
  const hasCompactionMaxFailures = value.compaction_max_consecutive_failures !== undefined && value.compaction_max_consecutive_failures !== null
  const hasToolResultRetention = value.tool_result_retention_enabled !== undefined && value.tool_result_retention_enabled !== null
  const hasMaxFragmentBytes = value.max_fragment_bytes !== undefined && value.max_fragment_bytes !== null
  const hasMaxTotalInjectedBytes = value.max_total_injected_bytes !== undefined && value.max_total_injected_bytes !== null
  const hasMaxFragments = value.max_fragments !== undefined && value.max_fragments !== null
  const hasMaxMetadataFieldBytes = value.max_metadata_field_bytes !== undefined && value.max_metadata_field_bytes !== null
  const hasMaxProviderInputBytes = value.max_provider_input_bytes !== undefined && value.max_provider_input_bytes !== null
  const effectiveCompactionEnabled = hasCompactionEnabled ? value.compaction_enabled : inherited.compaction_enabled ?? true
  const effectiveCompactionThreshold = hasCompactionThreshold ? value.compaction_threshold : inherited.compaction_threshold ?? 0.85
  const effectiveContextPressureScope = hasContextPressureScope ? value.context_pressure_scope : inherited.context_pressure_scope ?? 'body_after_prefix'
  const effectiveToolResultCleanupThreshold = hasToolResultCleanupThreshold ? value.tool_result_cleanup_threshold : inherited.tool_result_cleanup_threshold ?? 0.7
  const effectiveToolResultCleanupTarget = hasToolResultCleanupTarget ? value.tool_result_cleanup_target : inherited.tool_result_cleanup_target ?? 0.6
  const effectiveToolResultCleanupMinTokens = hasToolResultCleanupMinTokens ? value.tool_result_cleanup_min_tokens : inherited.tool_result_cleanup_min_tokens ?? 20_000
  const effectiveToolResultKeepRecent = hasToolResultKeepRecent ? value.tool_result_keep_recent : inherited.tool_result_keep_recent ?? 3
  const effectiveToolResultKeepRecentTokens = hasToolResultKeepRecentTokens ? value.tool_result_keep_recent_tokens : inherited.tool_result_keep_recent_tokens ?? 16_000
  const effectiveToolResultWarmSuffixTokens = hasToolResultWarmSuffixTokens ? value.tool_result_warm_suffix_tokens : inherited.tool_result_warm_suffix_tokens ?? 8_000
  const effectiveToolResultEagerMinTokens = hasToolResultEagerMinTokens ? value.tool_result_eager_min_tokens : inherited.tool_result_eager_min_tokens ?? 32_000
  const effectiveCompactionRecentTurns = hasCompactionRecentTurns ? value.compaction_recent_turns : inherited.compaction_recent_turns ?? 1
  const effectiveCompactionTargetMin = hasCompactionTargetMin ? value.compaction_target_min_ratio : inherited.compaction_target_min_ratio ?? 0.05
  const effectiveCompactionTargetMax = hasCompactionTargetMax ? value.compaction_target_max_ratio : inherited.compaction_target_max_ratio ?? 0.2
  const effectiveCompactionRecoveryBand = hasCompactionRecoveryBand ? value.compaction_recovery_band : inherited.compaction_recovery_band ?? 0.8
  const effectiveCompactionMaxFailures = hasCompactionMaxFailures ? value.compaction_max_consecutive_failures : inherited.compaction_max_consecutive_failures ?? 3
  const effectiveToolResultRetention = hasToolResultRetention ? value.tool_result_retention_enabled : inherited.tool_result_retention_enabled ?? (agent === 'ide' || agent === 'interactive_story')
  const effectiveMaxFragmentBytes = hasMaxFragmentBytes ? value.max_fragment_bytes : inherited.max_fragment_bytes ?? 256 * 1024
  const effectiveMaxTotalInjectedBytes = hasMaxTotalInjectedBytes ? value.max_total_injected_bytes : inherited.max_total_injected_bytes ?? 1024 * 1024
  const effectiveMaxFragments = hasMaxFragments ? value.max_fragments : inherited.max_fragments ?? 256
  const effectiveMaxMetadataFieldBytes = hasMaxMetadataFieldBytes ? value.max_metadata_field_bytes : inherited.max_metadata_field_bytes ?? 4 * 1024
  const effectiveMaxProviderInputBytes = hasMaxProviderInputBytes ? value.max_provider_input_bytes : inherited.max_provider_input_bytes ?? 4 * 1024 * 1024
  const isCompactionAgent = agent === 'context_compaction'
  const updatePressureRatio = (patch: Partial<AgentContextOverride>) => {
    onChange(normalizeContextPressurePatch(value, inherited, patch))
  }

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
            <Field label={t('agents.field.compactionThreshold')} inherited={!hasCompactionThreshold} onReset={hasCompactionThreshold ? () => updatePressureRatio({ compaction_threshold: null }) : undefined}>
              <Input
                type="number"
                aria-label={t('agents.field.compactionThreshold')}
                min={50}
                max={98}
                step={1}
                value={Math.round((effectiveCompactionThreshold ?? 0.85) * 100)}
                onChange={(e) => updatePressureRatio({ compaction_threshold: e.target.value === '' ? null : Number(e.target.value) / 100 })}
                className="h-7 flex-1 text-xs"
              />
            </Field>
            <Field label={t('agents.field.contextPressureScope')} inherited={!hasContextPressureScope} onReset={hasContextPressureScope ? () => onChange({ context_pressure_scope: null }) : undefined}>
              <Select value={effectiveContextPressureScope ?? 'body_after_prefix'} onValueChange={(scope) => onChange({ context_pressure_scope: scope as AgentContextOverride['context_pressure_scope'] })}>
                <SelectTrigger size="sm" className="min-w-0 flex-1" aria-label={t('agents.field.contextPressureScope')}>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectGroup>
                    <SelectItem value="body_after_prefix">{t('agents.option.pressureBodyAfterPrefix')}</SelectItem>
                    <SelectItem value="total">{t('agents.option.pressureTotal')}</SelectItem>
                  </SelectGroup>
                </SelectContent>
              </Select>
            </Field>
            <Field label={t('agents.field.toolResultCleanupThreshold')} inherited={!hasToolResultCleanupThreshold} onReset={hasToolResultCleanupThreshold ? () => updatePressureRatio({ tool_result_cleanup_threshold: null }) : undefined}>
              <Input
                type="number"
                aria-label={t('agents.field.toolResultCleanupThreshold')}
                min={1}
                max={98}
                step={1}
                value={Math.round((effectiveToolResultCleanupThreshold ?? 0.7) * 100)}
                onChange={(e) => updatePressureRatio({ tool_result_cleanup_threshold: e.target.value === '' ? null : Number(e.target.value) / 100 })}
                className="h-7 flex-1 text-xs"
              />
            </Field>
            <Field label={t('agents.field.toolResultCleanupTarget')} inherited={!hasToolResultCleanupTarget} onReset={hasToolResultCleanupTarget ? () => updatePressureRatio({ tool_result_cleanup_target: null }) : undefined}>
              <Input
                type="number"
                aria-label={t('agents.field.toolResultCleanupTarget')}
                min={1}
                max={98}
                step={1}
                value={Math.round((effectiveToolResultCleanupTarget ?? 0.6) * 100)}
                onChange={(e) => updatePressureRatio({ tool_result_cleanup_target: e.target.value === '' ? null : Number(e.target.value) / 100 })}
                className="h-7 flex-1 text-xs"
              />
            </Field>
            <Field label={t('agents.field.toolResultCleanupMinTokens')} inherited={!hasToolResultCleanupMinTokens} onReset={hasToolResultCleanupMinTokens ? () => onChange({ tool_result_cleanup_min_tokens: null }) : undefined}>
              <Input
                type="number"
                aria-label={t('agents.field.toolResultCleanupMinTokens')}
                min={0}
                max={16 * 1024 * 1024}
                step={1000}
                value={effectiveToolResultCleanupMinTokens ?? 20_000}
                onChange={(e) => onChange({ tool_result_cleanup_min_tokens: e.target.value === '' ? null : Number(e.target.value) })}
                className="h-7 flex-1 text-xs"
              />
            </Field>
            <Field label={t('agents.field.toolResultKeepRecent')} inherited={!hasToolResultKeepRecent} onReset={hasToolResultKeepRecent ? () => onChange({ tool_result_keep_recent: null }) : undefined}>
              <Input
                type="number"
                aria-label={t('agents.field.toolResultKeepRecent')}
                min={0}
                max={30}
                step={1}
                value={effectiveToolResultKeepRecent ?? 3}
                onChange={(e) => onChange({ tool_result_keep_recent: e.target.value === '' ? null : Number(e.target.value) })}
                className="h-7 flex-1 text-xs"
              />
            </Field>
            <Field label={t('agents.field.toolResultKeepRecentTokens')} inherited={!hasToolResultKeepRecentTokens} onReset={hasToolResultKeepRecentTokens ? () => onChange({ tool_result_keep_recent_tokens: null }) : undefined}>
              <Input
                type="number"
                aria-label={t('agents.field.toolResultKeepRecentTokens')}
                min={0}
                max={16 * 1024 * 1024}
                step={1000}
                value={effectiveToolResultKeepRecentTokens ?? 16_000}
                onChange={(e) => onChange({ tool_result_keep_recent_tokens: e.target.value === '' ? null : Number(e.target.value) })}
                className="h-7 flex-1 text-xs"
              />
            </Field>
            <Field label={t('agents.field.toolResultWarmSuffixTokens')} inherited={!hasToolResultWarmSuffixTokens} onReset={hasToolResultWarmSuffixTokens ? () => onChange({ tool_result_warm_suffix_tokens: null }) : undefined}>
              <Input
                type="number"
                aria-label={t('agents.field.toolResultWarmSuffixTokens')}
                min={0}
                max={16 * 1024 * 1024}
                step={1000}
                value={effectiveToolResultWarmSuffixTokens ?? 8_000}
                onChange={(e) => onChange({ tool_result_warm_suffix_tokens: e.target.value === '' ? null : Number(e.target.value) })}
                className="h-7 flex-1 text-xs"
              />
            </Field>
            <Field label={t('agents.field.toolResultEagerMinTokens')} inherited={!hasToolResultEagerMinTokens} onReset={hasToolResultEagerMinTokens ? () => onChange({ tool_result_eager_min_tokens: null }) : undefined}>
              <Input
                type="number"
                aria-label={t('agents.field.toolResultEagerMinTokens')}
                min={0}
                max={16 * 1024 * 1024}
                step={1000}
                value={effectiveToolResultEagerMinTokens ?? 32_000}
                onChange={(e) => onChange({ tool_result_eager_min_tokens: e.target.value === '' ? null : Number(e.target.value) })}
                className="h-7 flex-1 text-xs"
              />
            </Field>
            <Field label={t('agents.field.compactionRecoveryBand')} inherited={!hasCompactionRecoveryBand} onReset={hasCompactionRecoveryBand ? () => onChange({ compaction_recovery_band: null }) : undefined}>
              <Input
                type="number"
                aria-label={t('agents.field.compactionRecoveryBand')}
                min={10}
                max={100}
                step={1}
                value={Math.round((effectiveCompactionRecoveryBand ?? 0.8) * 100)}
                onChange={(e) => onChange({ compaction_recovery_band: e.target.value === '' ? null : Number(e.target.value) / 100 })}
                className="h-7 flex-1 text-xs"
              />
            </Field>
            <Field label={t('agents.field.compactionMaxFailures')} inherited={!hasCompactionMaxFailures} onReset={hasCompactionMaxFailures ? () => onChange({ compaction_max_consecutive_failures: null }) : undefined}>
              <Input
                type="number"
                aria-label={t('agents.field.compactionMaxFailures')}
                min={1}
                max={100}
                step={1}
                value={effectiveCompactionMaxFailures ?? 3}
                onChange={(e) => onChange({ compaction_max_consecutive_failures: e.target.value === '' ? null : Number(e.target.value) })}
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
        {!isCompactionAgent && <div className="mt-1">{t('agents.context.toolResultCleanupNote')}</div>}
        {!isCompactionAgent && <div className="mt-1">{t('agents.context.toolResultRetentionNote')}</div>}
        <div className="mt-1">{t('agents.context.assemblyBudgetNote')}</div>
      </div>
    </section>
  )
}

const pressureOrderingFallbackRatio = 0.85

// normalizeContextPressurePatch keeps the editor, persisted override, and Go
// runtime on one ordering invariant: cleanup target < cleanup trigger <
// compaction trigger. It adjusts only fields affected by the current edit, so
// inherited settings do not become unrelated child overrides.
export function normalizeContextPressurePatch(
  value: AgentContextOverride,
  inherited: AgentContextOverride,
  patch: Partial<AgentContextOverride>,
): Partial<AgentContextOverride> {
  const owns = (key: keyof AgentContextOverride) => Object.prototype.hasOwnProperty.call(patch, key)
  const effective = (key: keyof AgentContextOverride, fallback: number) => {
    const patched = patch[key]
    const inheritedValue = inherited[key]
    if (owns(key)) {
      return typeof patched === 'number' ? patched : (typeof inheritedValue === 'number' ? inheritedValue : fallback)
    }
    const current = value[key]
    if (typeof current === 'number') return current
    return typeof inheritedValue === 'number' ? inheritedValue : fallback
  }
  const result = { ...patch }
  const compaction = Math.max(0.5, Math.min(0.98, effective('compaction_threshold', 0.85)))
  let cleanup = Math.max(0.01, Math.min(0.98, effective('tool_result_cleanup_threshold', 0.7)))
  let target = Math.max(0.001, Math.min(0.98, effective('tool_result_cleanup_target', 0.6)))

  if (owns('compaction_threshold') && patch.compaction_threshold !== null) result.compaction_threshold = compaction
  if (cleanup >= compaction) {
    cleanup = compaction * pressureOrderingFallbackRatio
    result.tool_result_cleanup_threshold = cleanup
  } else if (owns('tool_result_cleanup_threshold') && patch.tool_result_cleanup_threshold !== null) {
    result.tool_result_cleanup_threshold = cleanup
  }
  if (target >= cleanup) {
    target = cleanup * pressureOrderingFallbackRatio
    result.tool_result_cleanup_target = target
  } else if (owns('tool_result_cleanup_target') && patch.tool_result_cleanup_target !== null) {
    result.tool_result_cleanup_target = target
  }
  return result
}
