import { useState } from 'react'
import { Brain, Check, ChevronDown, ChevronRight, FolderOpen, ImagePlus, ScrollText, Wrench } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import type { AgentModelOverride, AgentPromptBlocks, AgentPromptOverride, AgentPromptSource, AgentSkillOverride, AgentToolOverride, ResolvedAgentContextSettings, Settings } from '@/features/settings/types'
import { normalizeThinkingLevel, THINKING_LEVELS } from '@/features/settings/thinking-levels'
import type { SkillSummary } from '@/lib/api'
import { Field, SectionTitle, SwitchWithInheritance } from './agent-form-controls'
import { AGENTS, skillAgentFieldMatches, skillAvailableForAgent } from './agent-registry'
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
      <div className="grid gap-3 md:grid-cols-2">
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

export function AgentPromptSection({ value, inherited, builtin, blocks, sources, onChange }: {
  value: AgentPromptOverride
  inherited: AgentPromptOverride
  builtin: string
  blocks?: AgentPromptBlocks
  sources?: AgentPromptSource[]
  onChange: (patch: Partial<AgentPromptOverride>) => void
}) {
  const { t } = useTranslation()
  const promptSources = [...(sources?.length ? sources : fallbackPromptSources(blocks, builtin))]
    .sort((left, right) => Number(Boolean(right.editable)) - Number(Boolean(left.editable)))
  return (
    <section className="flex flex-col gap-3 border-b border-[var(--nova-border)] pb-5">
      <SectionTitle icon={ScrollText} title={t('agents.section.systemPrompt')} />
      <div className="flex flex-col gap-2">
        {promptSources.map((source) => (
          <PromptSourceBlock
            key={`${source.id}:${source.field ?? 'readonly'}`}
            source={source}
            value={value}
            inherited={inherited}
            onChange={onChange}
          />
        ))}
      </div>
      <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-2 text-[11px] leading-5 text-[var(--nova-text-faint)]">
        {t('agents.prompt.builtinNote')}
      </div>
    </section>
  )
}

function PromptSourceBlock({ source, value, inherited, onChange }: {
  source: AgentPromptSource
  value: AgentPromptOverride
  inherited: AgentPromptOverride
  onChange: (patch: Partial<AgentPromptOverride>) => void
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(Boolean(source.editable))
  const editableField = source.editable ? source.field : undefined
  const hasOverride = editableField ? hasPromptOverride(value[editableField]) : false
  const inheritedText = editableField ? inherited[editableField] : undefined
  const defaultContent = source.content ?? ''
  const effectiveContent = editableField
    ? (hasOverride ? value[editableField] ?? '' : (hasPromptOverride(inheritedText) ? inheritedText ?? '' : defaultContent))
    : defaultContent
  const title = promptSourceTitle(t, source)
  const badge = source.editable ? t('agents.prompt.badge.editable') : t('agents.prompt.badge.readonly')
  const content = effectiveContent.trim()

  return (
    <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)]">
      <button
        type="button"
        onClick={() => setOpen((current) => !current)}
        className="flex w-full items-center gap-2 px-3 py-2 text-left"
        aria-expanded={open}
      >
        {open ? <ChevronDown className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-muted)]" /> : <ChevronRight className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-muted)]" />}
        <span className="min-w-0 flex-1">
          <span className="block truncate text-[11px] font-medium text-[var(--nova-text)]">{title}</span>
          <span className="block truncate text-[10px] text-[var(--nova-text-faint)]">{source.source}</span>
        </span>
        <span className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] px-1.5 py-0.5 text-[10px] text-[var(--nova-text-faint)]">{badge}</span>
        {editableField && hasOverride && <span className="rounded-[var(--nova-radius)] bg-[var(--nova-active)] px-1.5 py-0.5 text-[10px] text-[var(--nova-text-muted)]">{t('agents.badge.overridden')}</span>}
      </button>
      {open && (
        <div className="border-t border-[var(--nova-border)] p-3">
          {editableField ? (
            <Field label={title} inherited={!hasOverride} onReset={hasOverride ? () => onChange({ [editableField]: '' }) : undefined}>
              <Textarea
                autoResize
                value={effectiveContent}
                aria-label={title}
                placeholder={t('agents.prompt.placeholder')}
                onChange={(e) => onChange({ [editableField]: e.target.value })}
                className="min-h-36 resize-y text-xs leading-5"
              />
            </Field>
          ) : content ? (
            <pre className="max-h-56 overflow-auto whitespace-pre-wrap text-[11px] leading-5 text-[var(--nova-text-faint)]">{effectiveContent}</pre>
          ) : (
            <div className="text-[11px] text-[var(--nova-text-faint)]">{t('agents.prompt.empty')}</div>
          )}
        </div>
      )}
    </div>
  )
}

function fallbackPromptSources(blocks?: AgentPromptBlocks, builtin?: string): AgentPromptSource[] {
  return [
    blocks?.runtime_contract ? {
      id: 'runtime_contract',
      title: 'Runtime Contract',
      source: 'Denova runtime',
      content: blocks.runtime_contract,
    } : null,
    blocks?.output_protocol ? {
      id: 'output_protocol',
      title: 'Output Format',
      source: 'Denova runtime',
      content: blocks.output_protocol,
    } : null,
    {
      id: 'flow',
      title: 'Flow Rules',
      source: 'Denova built-in',
      content: blocks?.editable_system_prompt || builtin || '',
      editable: true,
      field: 'flow_prompt' as const,
    },
    {
      id: 'custom',
      title: 'Custom Rules',
      source: 'user/workspace config',
      content: '',
      editable: true,
      field: 'system_prompt' as const,
    },
  ].filter(Boolean) as AgentPromptSource[]
}

function promptSourceTitle(t: ReturnType<typeof useTranslation>['t'], source: AgentPromptSource) {
  const key = `agents.prompt.source.${source.id}`
  const translated = t(key)
  return translated === key ? source.title : translated
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
      ) : <div className="grid gap-2 lg:grid-cols-2">
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
            <div key={tool.key} className="flex min-h-16 min-w-0 flex-col items-stretch gap-3 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 py-2 sm:flex-row sm:items-center">
              <Icon className="h-4 w-4 shrink-0 text-[var(--nova-text-muted)]" />
              <div className="min-w-0 flex-1">
                <div className="flex min-w-0 items-center gap-2">
                  <span className="min-w-0 truncate font-medium">{t(tool.titleKey)}</span>
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

export function AgentSkillSection({ agent, skills, value, effective, onChange }: {
  agent: VisibleAgentKey
  skills: SkillSummary[]
  value: AgentSkillOverride
  effective: Settings['agent_skills']
  onChange: (name: string, value: boolean | null) => void
}) {
  const { t } = useTranslation()
  return (
    <section className="flex flex-col gap-3 border-b border-[var(--nova-border)] pb-5">
      <SectionTitle icon={FolderOpen} title={t('agents.section.skills')} />
      {skills.length === 0 ? (
        <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 py-3 text-[11px] text-[var(--nova-text-faint)]">
          {t('agents.skills.empty')}
        </div>
      ) : (
        <div className="grid gap-2 lg:grid-cols-2">
          {skills.map((skill) => {
            const explicit = value[skill.name]
            const inherited = explicit === undefined
            const current = inherited ? skillAvailableForAgent(skill, agent, effective) : explicit
            const defaultAvailable = skillAgentFieldMatches(skill.agent, agent)
            const targetAgents = (skill.agent || '')
              .split(/[,\s;]+/)
              .map((key) => key.trim())
              .filter(Boolean)
              .map((key) => {
                const titleKey = AGENTS.find((candidate) => candidate.key === key)?.titleKey
                return titleKey ? t(titleKey) : key
              })
              .join(', ')
            return (
              <div key={`${skill.scope}:${skill.name}`} className="flex min-h-16 min-w-0 flex-col items-stretch gap-3 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 py-2 sm:flex-row sm:items-center">
                <FolderOpen className="h-4 w-4 shrink-0 text-[var(--nova-text-muted)]" />
                <div className="min-w-0 flex-1">
                  <div className="flex min-w-0 items-center gap-2">
                    <span className="min-w-0 truncate font-mono font-medium">/{skill.name}</span>
                    <span className={`shrink-0 rounded px-1.5 py-0.5 text-[10px] ${current ? 'bg-[var(--nova-success-bg)] text-[var(--nova-success)]' : 'bg-[var(--nova-danger-bg)] text-[var(--nova-danger)]'}`}>
                      {current ? t('agents.skills.available') : t('agents.skills.unavailable')}
                    </span>
                  </div>
                  <div className="mt-0.5 truncate text-[11px] text-[var(--nova-text-faint)]">{skill.description}</div>
                  <div className="mt-0.5 truncate text-[10px] text-[var(--nova-text-faint)]">
                    {defaultAvailable ? t('agents.skills.defaultAvailable') : t('agents.skills.defaultUnavailable')}
                    {targetAgents ? ` · ${targetAgents}` : ''}
                  </div>
                </div>
                <SwitchWithInheritance
                  checked={Boolean(current)}
                  onChange={(checked) => onChange(skill.name, checked)}
                  ariaLabel={`/${skill.name}`}
                  inherited={inherited}
                  onReset={!inherited ? () => onChange(skill.name, null) : undefined}
                />
              </div>
            )
          })}
        </div>
      )}
      <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-2 text-[11px] leading-5 text-[var(--nova-text-faint)]">
        {t('agents.skills.note')}
      </div>
    </section>
  )
}

export function AgentBuiltInCapabilitySection({ agent }: { agent: VisibleAgentKey }) {
  const { t } = useTranslation()
  const rows = builtInCapabilityRows(agent, t)
  return (
    <section className="flex flex-col gap-3 border-b border-[var(--nova-border)] pb-5">
      <SectionTitle icon={Wrench} title={t('agents.section.builtIn')} />
      <div className="grid gap-2 md:grid-cols-2">
        {rows.map((row) => (
          <div key={row.title} className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 py-2">
            <div className="font-medium text-[var(--nova-text)]">{row.title}</div>
            <div className="mt-1 text-[11px] leading-5 text-[var(--nova-text-faint)]">{row.value}</div>
          </div>
        ))}
      </div>
      <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-2 text-[11px] leading-5 text-[var(--nova-text-faint)]">
        {t('agents.builtIn.note')}
      </div>
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
      <div className="grid gap-2 md:grid-cols-3">
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

function builtInCapabilityRows(agent: VisibleAgentKey, t: (key: string) => string): Array<{ title: string; value: string }> {
  void agent
  void t
  return []
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
