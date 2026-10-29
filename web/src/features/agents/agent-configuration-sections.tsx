import { Brain, Check, FolderOpen, ImagePlus, Wrench } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import type { AgentModelOverride, AgentPromptOverride, AgentToolOverride, ResolvedAgentContextSettings, Settings } from '@/features/settings/types'
import { normalizeThinkingLevel, THINKING_LEVELS } from '@/features/settings/thinking-levels'
import { Field, SectionTitle, SwitchWithInheritance } from './agent-form-controls'
import type { AgentToolDefinition, ToolKey, VisibleAgentKey } from './agent-registry'

export function AgentModelSection({ value, inherited, profiles, onChange }: {
  value: AgentModelOverride
  inherited: AgentModelOverride
  profiles: Array<{ id: string; label: string }>
  onChange: (patch: Partial<AgentModelOverride>) => void
}) {
  const { t } = useTranslation()
  const hasProfile = hasTextOverride(value.profile_id)
  const hasThinkingLevel = hasTextOverride(value.thinking_level)
  const effectiveProfile = hasProfile ? value.profile_id || 'default' : inherited.profile_id || 'default'
  const effectiveThinkingLevel = normalizeThinkingLevel(hasThinkingLevel ? value.thinking_level : inherited.thinking_level) ?? 'default'

  return (
    <section className="flex flex-col gap-3 border-b border-[var(--nova-border)] pb-5">
      <SectionTitle icon={Brain} title={t('agents.section.model')} />
      <div className="grid gap-3 @xl/agent-settings:grid-cols-2">
        <Field label={t('agents.field.modelProfile')} inherited={!hasProfile} onReset={hasProfile ? () => onChange({ profile_id: '' }) : undefined}>
          <Select value={effectiveProfile} onValueChange={(profileID) => onChange({ profile_id: profileID })}>
            <SelectTrigger size="sm" className="min-w-0 flex-1" aria-label={t('agents.field.modelProfile')}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectGroup>
                {profiles.map((profile) => <SelectItem key={profile.id} value={profile.id}>{profile.label}</SelectItem>)}
              </SelectGroup>
            </SelectContent>
          </Select>
        </Field>
        <Field label={t('agents.field.thinkingLevel')} inherited={!hasThinkingLevel} onReset={hasThinkingLevel ? () => onChange({ thinking_level: '' }) : undefined}>
          <Select value={effectiveThinkingLevel} onValueChange={(level) => onChange({ thinking_level: level })}>
            <SelectTrigger size="sm" className="min-w-0 flex-1" aria-label={t('agents.field.thinkingLevel')}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectGroup>
                {THINKING_LEVELS.map((level) => (
                  <SelectItem key={level} value={level}>{t(`agents.thinkingLevel.${level}`)}</SelectItem>
                ))}
              </SelectGroup>
            </SelectContent>
          </Select>
        </Field>
      </div>
    </section>
  )
}

/** Selects the image provider profile used by generate_image. This is separate
 * from the language model above, which turns creative context into a prompt. */
export function AgentImageModelSection({ value, inherited, profiles, onChange }: {
  value: string
  inherited: string
  profiles: Array<{ id: string; label: string }>
  onChange: (profileID: string) => void
}) {
  const { t } = useTranslation()
  const hasOverride = hasTextOverride(value)
  const effectiveProfile = hasOverride ? value : inherited || 'default'

  return (
    <section className="flex flex-col gap-3 border-b border-[var(--nova-border)] pb-5">
      <SectionTitle icon={ImagePlus} title={t('agents.section.imageModel')} />
      <Field
        label={t('agents.field.imageModelProfile')}
        inherited={!hasOverride}
        onReset={hasOverride ? () => onChange('') : undefined}
      >
        <Select value={effectiveProfile} onValueChange={onChange}>
          <SelectTrigger size="sm" className="min-w-0 flex-1" aria-label={t('agents.field.imageModelProfile')}>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectGroup>
              {profiles.map((profile) => <SelectItem key={profile.id} value={profile.id}>{profile.label}</SelectItem>)}
            </SelectGroup>
          </SelectContent>
        </Select>
      </Field>
      <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-2 text-[11px] leading-5 text-[var(--nova-text-faint)]">
        {t('agents.imageModel.note')}
      </div>
    </section>
  )
}

export function AgentToolSection({ value, rows, onChange }: {
  value: AgentToolOverride
  rows: AgentToolDefinition[]
  onChange: (key: ToolKey, value: boolean | null) => void
}) {
  const { t } = useTranslation()
  return (
    <section className="flex flex-col gap-3 border-b border-[var(--nova-border)] pb-5">
      <SectionTitle icon={Wrench} title={t('agents.section.tools')} />
      {rows.length === 0 ? (
        <div className="rounded-[var(--nova-radius)] border border-dashed border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-3 text-[11px] text-[var(--nova-text-faint)]">—</div>
      ) : <div className="flex flex-col">
        {rows.map((tool) => {
          const Icon = tool.icon
          const explicit = value[tool.key]
          const inherited = explicit === undefined
          const current = inherited ? tool.allowed : Boolean(explicit)
          const isRuntimeCheck = tool.availability === 'runtime_check'
          const isUnavailable = tool.availability === 'unavailable'
          const availabilityLabel = isRuntimeCheck
            ? t('agents.tool.availability.runtimeCheck')
            : (isUnavailable ? t('agents.skills.unavailable') : '')
          const availabilityHint = isUnavailable && tool.unavailableReasonKey
            ? t(tool.unavailableReasonKey)
            : availabilityLabel
          return (
            <div key={tool.key} className="flex min-h-16 min-w-0 flex-wrap items-center gap-3 border-b border-[var(--nova-border)] py-3 last:border-b-0">
              <Icon className="h-4 w-4 shrink-0 text-[var(--nova-text-muted)]" />
              <div className="min-w-0 flex-1 basis-40">
                <div className="flex min-w-0 items-center gap-2">
                  <span className="min-w-0 break-words font-medium">{t(tool.titleKey)}</span>
                  {(isRuntimeCheck || isUnavailable) && (
                    <span
                      className={`shrink-0 rounded px-1.5 py-0.5 text-[10px] ${isUnavailable ? 'bg-[var(--nova-danger-bg)] text-[var(--nova-danger)]' : 'bg-[var(--nova-warning-bg)] text-[var(--nova-warning)]'}`}
                    >
                      {availabilityLabel}
                    </span>
                  )}
                </div>
                <div className="mt-0.5 truncate text-[11px] text-[var(--nova-text-faint)]">
                  {tool.toolNames.length > 0 ? tool.toolNames.join(' / ') : t(tool.subtitleKey)}
                </div>
                {isUnavailable && availabilityHint && (
                  <div className="mt-0.5 truncate text-[10px] text-[var(--nova-danger)]">{availabilityHint}</div>
                )}
              </div>
              <SwitchWithInheritance
                checked={Boolean(current)}
                onChange={(checked) => onChange(tool.key, checked)}
                ariaLabel={t(tool.titleKey)}
                inherited={inherited}
                onReset={!inherited ? () => onChange(tool.key, null) : undefined}
              />
            </div>
          )
        })}
      </div>}
    </section>
  )
}

export function AgentContextSection({ agent, effective, resolved }: {
  agent: VisibleAgentKey
  effective: Settings
  resolved?: ResolvedAgentContextSettings
}) {
  const { t } = useTranslation()
  const rows = contextRowsFor(agent, effective, resolved, t)
  return (
    <section className="flex flex-col gap-3 pb-5">
      <SectionTitle icon={FolderOpen} title={t('agents.section.context')} />
      <div className="grid gap-2 @xl/agent-settings:grid-cols-3">
        {rows.map((row) => (
          <div key={row.title} className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 py-2">
            <div className="flex items-center gap-1.5 text-[var(--nova-text)]">
              <Check className="h-3.5 w-3.5 text-[var(--nova-accent-green)]" />
              <span className="font-medium">{row.title}</span>
            </div>
            <div className="mt-1 truncate text-[11px] text-[var(--nova-text-faint)]">{row.value}</div>
          </div>
        ))}
      </div>
    </section>
  )
}

function contextRowsFor(
  agent: VisibleAgentKey,
  effective: Settings,
  resolved: ResolvedAgentContextSettings | undefined,
  t: (key: string, options?: Record<string, unknown>) => string,
) {
  const compactionValue = resolved
    ? t('agents.context.compactionValue', { threshold: Math.round(resolved.compaction_threshold * 100) })
    : '-'
  if (agent === 'ide') {
    return [
      { title: t('agents.context.currentBook'), value: 'workspace' },
      { title: t('agents.context.defaultTeller'), value: effective.ide_story_teller_id || 'rhythm' },
      { title: t('agents.context.sessionContext'), value: compactionValue },
    ]
  }
  if (agent === 'interactive_story') {
    return [
      { title: t('agents.context.storyState'), value: 'story jsonl' },
      { title: t('agents.context.teller'), value: t('agents.context.currentStoryTeller') },
      { title: t('agents.context.sessionContext'), value: compactionValue },
    ]
  }
  return [
    { title: t('agents.context.inputSource'), value: t('agents.context.inputSourceValue') },
    { title: t('agents.context.outputShape'), value: t('agents.context.outputShapeValue') },
    { title: t('agents.context.historyBoundary'), value: compactionValue },
  ]
}

function hasTextOverride(value?: string) {
  return value !== undefined && value !== ''
}

function hasPromptOverride(value?: string) {
  return value !== undefined && value.trim() !== ''
}

export function mergeAgentModelOverride(parent: AgentModelOverride, child: AgentModelOverride): AgentModelOverride {
  return {
    profile_id: child.profile_id || parent.profile_id,
    temperature: child.temperature ?? parent.temperature,
    thinking_level: child.thinking_level || parent.thinking_level,
  }
}

export function mergeAgentPromptOverride(parent: AgentPromptOverride, child: AgentPromptOverride): AgentPromptOverride {
  return {
    flow_prompt: hasPromptOverride(child.flow_prompt) ? child.flow_prompt : parent.flow_prompt,
    system_prompt: hasPromptOverride(child.system_prompt) ? child.system_prompt : parent.system_prompt,
  }
}
