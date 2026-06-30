import { useCallback, useEffect, useMemo, useState } from 'react'
import type { ElementType, ReactNode } from 'react'
import { Bot, Brain, Check, ChevronDown, ChevronRight, Edit3, FolderOpen, Loader2, PanelLeft, Plus, Save, ScrollText, Trash2, Wrench, X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { ConfigManagerChat } from '@/components/Chat/ConfigManagerChat'
import { InlineErrorNotice } from '@/components/common/inline-error-notice'
import { AdaptiveSurface } from '@/components/layout/adaptive-surface'
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from '@/components/ui/alert-dialog'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { fetchSettings, updateUserSettings, updateWorkspaceSettings } from '@/features/settings/api'
import type { AgentContextOverride, AgentModelOverride, AgentPromptBlocks, AgentPromptOverride, AgentPromptSource, AgentSkillOverride, AgentToolOverride, LayeredSettings, ModelProfileSettings, Settings, SettingsLayer, SubAgentConfig } from '@/features/settings/types'
import { modelProfileID, modelProfileLabel, modelProfilesWithDefault } from '@/features/settings/model-profiles'
import { settingsForLayer, settingsRevisionForLayer, useAutoSaveSettings } from '@/features/settings/use-auto-save-settings'
import { getSkills } from '@/lib/api'
import type { SkillSummary } from '@/lib/api'
import { AGENTS, DEEP_AGENT_PARENT_KEYS, FALLBACK_AGENT_TOOL_VALUES, TOOL_ROWS, resolveEffectiveTools, skillAgentFieldMatches, skillAvailableForAgent } from './agent-registry'
import type { AgentToolDefinition, AgentViewDefinition, DeepAgentParentKey, ToolKey, VisibleAgentKey } from './agent-registry'

const fieldCls = 'nova-field min-h-7 w-full min-w-0 flex-1 rounded-[var(--nova-radius)] border px-2.5 py-1.5 outline-none placeholder:text-[var(--nova-text-faint)] focus:border-[var(--nova-field-focus-border)] focus:bg-[var(--nova-surface-3)]'
const tabCls = 'nova-nav-item rounded-[var(--nova-radius)] px-2.5 py-1 text-xs'
let nextSettingsEventSourceID = 1

export function AgentsView({ onClose }: { onClose?: () => void }) {
  const { t } = useTranslation()
  const [layered, setLayered] = useState<LayeredSettings | null>(null)
  const [activeLayer, setActiveLayer] = useState<SettingsLayer>('user')
  const [activeAgent, setActiveAgent] = useState<VisibleAgentKey>('ide')
  const [draft, setDraft] = useState<Settings>({})
  const [skills, setSkills] = useState<SkillSummary[]>([])
  const [agentChatOpen, setAgentChatOpen] = useState(false)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [settingsEventSource] = useState(() => {
    const source = `agents-view-${nextSettingsEventSourceID}`
    nextSettingsEventSourceID += 1
    return source
  })

  const load = useCallback(async () => {
    try {
      const data = await fetchSettings()
      setLayered(data)
      setDraft(settingsForLayer(data, activeLayer))
    } catch (e) {
      setError((e as Error).message)
    }
  }, [activeLayer])

  useEffect(() => { void load() }, [load])

  useEffect(() => {
    const onSettingsUpdated = (event: Event) => {
      const source = (event as CustomEvent<{ source?: string }>).detail?.source
      if (source === settingsEventSource) return
      void load()
    }
    window.addEventListener('nova:settings-updated', onSettingsUpdated)
    return () => window.removeEventListener('nova:settings-updated', onSettingsUpdated)
  }, [load, settingsEventSource])

  useEffect(() => {
    let cancelled = false
    const loadSkills = () => {
      getSkills()
        .then((snapshot) => {
          if (!cancelled) setSkills(snapshot.skills.filter((skill) => skill.active))
        })
        .catch((error) => {
          if (!cancelled) console.warn('[agents] load skills failed', error)
        })
    }
    loadSkills()
    window.addEventListener('nova:skills-updated', loadSkills)
    return () => {
      cancelled = true
      window.removeEventListener('nova:skills-updated', loadSkills)
    }
  }, [])

  useEffect(() => {
    if (!layered) return
    setDraft(settingsForLayer(layered, activeLayer))
  }, [activeLayer])

  const effective = layered?.effective ?? {}
  const selected = AGENTS.find((agent) => agent.key === activeAgent) ?? AGENTS[0]
  const profileOptions = useMemo(() => buildProfileOptions(draft, effective, t), [draft, effective, t])
  const modelValue = draft.agent_models?.[activeAgent] ?? {}
  const inheritedModel = mergeAgentModelOverride(effective.agent_models?.default ?? {}, effective.agent_models?.[activeAgent] ?? {})
  const promptValue = draft.agent_prompts?.[activeAgent] ?? {}
  const inheritedPrompt = mergeAgentPromptOverride(effective.agent_prompts?.default ?? {}, effective.agent_prompts?.[activeAgent] ?? {})
  const builtinPrompt = layered?.builtin_agent_prompts?.[activeAgent]?.system_prompt ?? ''
  const builtinBlocks = layered?.builtin_agent_prompt_blocks?.[activeAgent]
  const promptSources = layered?.builtin_agent_prompt_sources?.[activeAgent]?.sources
  const toolValue = draft.agent_tools?.[activeAgent] ?? {}
  const inheritedTools = effective.agent_tools?.[activeAgent] ?? FALLBACK_AGENT_TOOL_VALUES[activeAgent]
  const effectiveTools = resolveEffectiveTools(effective.agent_tools?.default ?? {}, inheritedTools)
  const skillValue = draft.agent_skills?.[activeAgent] ?? {}
  const contextValue = draft.agent_context?.[activeAgent] ?? {}
  const inheritedContext = mergeAgentContextOverride(effective.agent_context?.default ?? {}, effective.agent_context?.[activeAgent] ?? {})
  const generalSubAgents = draft.general_sub_agents ?? {}
  const previewGeneralSubAgents = useMemo(() => previewGeneralSubAgentSettings(layered, activeLayer, draft), [activeLayer, draft, layered])
  const subAgents = draft.sub_agents ?? []
  const configManagerWorkspaceKey = layered?.paths.workspace_config || layered?.paths.user_config || 'agents'
  const configManagerContext = useMemo(() => ({
    active_settings_layer: activeLayer,
    active_agent: activeAgent,
    active_agent_title: t(selected.titleKey),
    write_scope_required: 'true',
    write_scope_hint: activeLayer,
  }), [activeAgent, activeLayer, selected.titleKey, t])

  const saveDraft = useCallback(async (settings: Settings, baseRevision?: string) => {
    const updater = activeLayer === 'user' ? updateUserSettings : updateWorkspaceSettings
    return baseRevision ? updater(settings, baseRevision) : updater(settings)
  }, [activeLayer])

  const applySavedSettings = useCallback((next: LayeredSettings) => {
    setLayered(next)
    if (typeof window !== 'undefined') {
      window.dispatchEvent(new CustomEvent('nova:settings-updated', { detail: { source: settingsEventSource } }))
    }
  }, [settingsEventSource])

  const onSave = async () => {
    setSaving(true)
    setError(null)
    try {
      const next = await saveDraft(draft, settingsRevisionForLayer(layered, activeLayer))
      applySavedSettings(next)
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setSaving(false)
    }
  }

  const reloadAfterAgentMutation = useCallback(() => {
    void load()
    if (typeof window !== 'undefined') {
      window.dispatchEvent(new CustomEvent('nova:settings-updated', { detail: { source: settingsEventSource } }))
    }
  }, [load, settingsEventSource])

  const setAgentModel = (patch: Partial<AgentModelOverride>) => {
    setDraft((current) => ({
      ...current,
      agent_models: {
        ...(current.agent_models ?? {}),
        [activeAgent]: { ...(current.agent_models?.[activeAgent] ?? {}), ...patch },
      },
    }))
  }

  const setAgentTool = (key: ToolKey, value: boolean | null) => {
    setDraft((current) => ({
      ...current,
      agent_tools: {
        ...(current.agent_tools ?? {}),
        [activeAgent]: { ...(current.agent_tools?.[activeAgent] ?? {}), [key]: value },
      },
    }))
  }

  const setAgentPrompt = (patch: Partial<AgentPromptOverride>) => {
    setDraft((current) => ({
      ...current,
      agent_prompts: {
        ...(current.agent_prompts ?? {}),
        [activeAgent]: { ...(current.agent_prompts?.[activeAgent] ?? {}), ...patch },
      },
    }))
  }

  const setAgentSkill = (name: string, value: boolean | null) => {
    setDraft((current) => {
      const nextAgentSkills = { ...(current.agent_skills ?? {}) }
      const nextOverrides: AgentSkillOverride = { ...(nextAgentSkills[activeAgent] ?? {}) }
      if (value === null) {
        delete nextOverrides[name]
      } else {
        nextOverrides[name] = value
      }
      nextAgentSkills[activeAgent] = nextOverrides
      return { ...current, agent_skills: nextAgentSkills }
    })
  }

  const setAgentContext = (patch: Partial<AgentContextOverride>) => {
    setDraft((current) => ({
      ...current,
      agent_context: {
        ...(current.agent_context ?? {}),
        [activeAgent]: { ...(current.agent_context?.[activeAgent] ?? {}), ...patch },
      },
    }))
  }

  const setSubAgents = (updater: (current: SubAgentConfig[]) => SubAgentConfig[]) => {
    setDraft((current) => ({
      ...current,
      sub_agents: updater(current.sub_agents ?? []),
    }))
  }

  const setGeneralSubAgent = (agent: DeepAgentParentKey, value: boolean | null) => {
    setDraft((current) => {
      const next = { ...(current.general_sub_agents ?? {}) }
      if (value === null) delete next[agent]
      else next[agent] = value
      return { ...current, general_sub_agents: next }
    })
  }

  useAutoSaveSettings({
    draft,
    saved: layered ? settingsForLayer(layered, activeLayer) : {},
    baseRevision: settingsRevisionForLayer(layered, activeLayer),
    ready: Boolean(layered),
    save: saveDraft,
    onSavingChange: setSaving,
    onSaved: applySavedSettings,
    onError: setError,
  })

  return (
    <div className="flex h-full min-h-0 w-full flex-col bg-[var(--nova-bg)] text-[var(--nova-text)]">
      <div className="nova-topbar flex min-h-10 shrink-0 flex-nowrap max-md:flex-wrap items-center gap-2 overflow-x-auto max-md:overflow-x-hidden border-b px-3 py-1.5 text-xs sm:px-4">
        <Bot className="h-3.5 w-3.5 text-[var(--nova-text-muted)]" />
        <span className="shrink-0 font-medium">Agents</span>
        <div className="flex shrink-0 gap-1 border-l border-[var(--nova-border)] pl-2 sm:ml-3 sm:pl-3">
          {(['user', 'workspace'] as SettingsLayer[]).map((layer) => (
            <button
              key={layer}
              type="button"
              onClick={() => setActiveLayer(layer)}
              className={`${tabCls} ${activeLayer === layer ? 'is-active' : 'bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)]'}`}
            >
              {layer === 'workspace' ? t('agents.layer.workspace') : t('agents.layer.user')}
            </button>
          ))}
        </div>
        <button
          type="button"
          onClick={onSave}
          disabled={saving}
          className="nova-nav-item ml-auto inline-flex shrink-0 items-center gap-1.5 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-active)] px-3 py-1 text-[var(--nova-text)] disabled:opacity-50"
        >
          {saving ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Save className="h-3.5 w-3.5" />}
          {t('common.save')}
        </button>
        <button
          type="button"
          onClick={() => setAgentChatOpen((value) => !value)}
          className={`nova-nav-item inline-flex shrink-0 items-center gap-1.5 rounded-[var(--nova-radius)] border border-[var(--nova-border)] px-3 py-1 ${agentChatOpen ? 'is-active' : 'bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)]'}`}
          aria-pressed={agentChatOpen}
        >
          <Bot className="h-3.5 w-3.5" />
          {t('agents.configAgent.button')}
        </button>
        {onClose && (
          <button type="button" onClick={onClose} className="nova-nav-item rounded p-1" aria-label={t('agents.close')} title={t('agents.close')}>
            <X className="h-3.5 w-3.5" />
          </button>
        )}
      </div>

      {error && <InlineErrorNotice className="mx-3 mt-2" message={error} title={t('agents.saveError')} />}

      <AdaptiveSurface
        left={{
          id: 'agents-list',
          title: 'Agents',
          side: 'left',
          icon: <Bot className="h-4 w-4" />,
          content: <div className="h-full min-h-0 overflow-y-auto bg-[var(--nova-surface-2)] p-3"><AgentList active={activeAgent} onSelect={setActiveAgent} /></div>,
          desktopClassName: 'min-h-0 border-r border-[var(--nova-border)]',
          mobileClassName: 'w-[min(88vw,340px)]',
        }}
        right={agentChatOpen ? {
          id: 'agents-config-manager',
          title: t('agents.configAgent.title'),
          side: 'right',
          icon: <Bot className="h-4 w-4" />,
          content: (
            <div className="h-full min-h-0 bg-[var(--nova-surface)]">
              <ConfigManagerChat
                workspace={configManagerWorkspaceKey}
                origin="agents"
                resourceId={`${activeLayer}:${activeAgent}`}
                context={configManagerContext}
                onMutated={reloadAfterAgentMutation}
              />
            </div>
          ),
          desktopClassName: 'min-h-0 border-l border-[var(--nova-border)]',
          mobileClassName: 'w-[min(92vw,420px)]',
        } : undefined}
        className="flex-1 text-xs"
        mainClassName="min-h-0 min-w-0"
        desktopGridClassName={agentChatOpen ? 'grid-cols-[18rem_minmax(0,1fr)_minmax(320px,28rem)]' : 'grid-cols-[18rem_minmax(0,1fr)]'}
      >
        {({ openLeft, openRight }) => (
          <main className="h-full min-h-0 overflow-y-auto overflow-x-hidden">
            <div className="sticky top-0 z-10 flex h-10 items-center gap-2 border-b border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 md:hidden">
              <button type="button" className="nova-icon-button flex h-8 w-8 items-center justify-center rounded-[var(--nova-radius)] border border-[var(--nova-border)] text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]" aria-label={t('workbench.mobile.openSidePanel', { label: 'Agents' })} onClick={openLeft}>
                <PanelLeft className="h-4 w-4" />
              </button>
              <span className="min-w-0 truncate text-[11px] text-[var(--nova-text-muted)]">{t(selected.titleKey)}</span>
              {agentChatOpen && (
                <button type="button" className="nova-icon-button ml-auto flex h-8 w-8 items-center justify-center rounded-[var(--nova-radius)] border border-[var(--nova-border)] text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]" aria-label={t('workbench.mobile.openSidePanel', { label: t('agents.configAgent.title') })} onClick={openRight}>
                  <Bot className="h-4 w-4" />
                </button>
              )}
            </div>
            <div className="mx-auto flex w-full min-w-0 max-w-5xl flex-col gap-5 px-4 py-5 sm:px-6">
              <AgentHeader agent={selected} />
              <AgentModelSection
                value={modelValue}
                inherited={inheritedModel}
                profiles={profileOptions}
                onChange={setAgentModel}
              />
              <AgentPromptSection
                value={promptValue}
                inherited={inheritedPrompt}
                builtin={builtinPrompt}
                blocks={builtinBlocks}
                sources={promptSources}
                onChange={setAgentPrompt}
              />
              <AgentRuntimeContextSection
                agent={activeAgent}
                value={contextValue}
                inherited={inheritedContext}
                onChange={setAgentContext}
              />
              {selected.capabilityMode === 'tools' ? (
                <>
                  <AgentToolSection
                    agent={activeAgent}
                    value={toolValue}
                    effective={effectiveTools}
                    onChange={setAgentTool}
                  />
                  {isDeepAgentParent(activeAgent) && (
                    <AgentSubAgentSection
                      agent={activeAgent}
                      inheritedModel={inheritedModel}
                      generalSettings={generalSubAgents}
                      effectiveGeneralSettings={previewGeneralSubAgents}
                      subAgents={subAgents}
                      effectiveSubAgents={effective.sub_agents ?? []}
                      profiles={profileOptions}
                      onGeneralChange={setGeneralSubAgent}
                      onChange={setSubAgents}
                    />
                  )}
                  {effectiveTools.skills && (
                    <AgentSkillSection
                      agent={activeAgent}
                      skills={skills}
                      value={skillValue}
                      effective={effective.agent_skills}
                      onChange={setAgentSkill}
                    />
                  )}
                </>
              ) : selected.capabilityMode === 'built_in' ? (
                <AgentBuiltInCapabilitySection agent={selected.key} />
              ) : (
                <AgentModelOnlySection />
              )}
              <AgentContextSection agent={selected.key} effective={effective} />
            </div>
          </main>
        )}
      </AdaptiveSurface>
    </div>
  )
}

function AgentList({ active, onSelect }: { active: VisibleAgentKey; onSelect: (agent: VisibleAgentKey) => void }) {
  const { t } = useTranslation()
  const groups = AGENTS.reduce<Array<{ group: string; agents: typeof AGENTS }>>((acc, agent) => {
    const last = acc[acc.length - 1]
    if (last?.group === agent.groupKey) last.agents.push(agent)
    else acc.push({ group: agent.groupKey, agents: [agent] })
    return acc
  }, [])

  return (
    <nav className="space-y-4">
      {groups.map((group) => (
        <div key={group.group}>
          <div className="mb-1.5 px-2 text-[11px] font-medium text-[var(--nova-text-faint)]">{t(group.group)}</div>
          <div className="space-y-1">
            {group.agents.map((agent) => {
              const Icon = agent.icon
              return (
                <button
                  key={agent.key}
                  type="button"
                  onClick={() => onSelect(agent.key)}
                  className={`nova-nav-item flex w-full items-center gap-2 rounded-[var(--nova-radius)] px-2.5 py-2 text-left ${active === agent.key ? 'is-active' : ''}`}
                >
                  <Icon className="h-4 w-4 shrink-0 text-[var(--nova-text-muted)]" />
                  <span className="min-w-0">
                    <span className="block truncate font-medium text-[var(--nova-text)]">{t(agent.titleKey)}</span>
                    <span className="block truncate text-[11px] text-[var(--nova-text-faint)]">{t(agent.subtitleKey)}</span>
                  </span>
                </button>
              )
            })}
          </div>
        </div>
      ))}
    </nav>
  )
}

function AgentHeader({ agent }: { agent: AgentViewDefinition }) {
  const { t } = useTranslation()
  const Icon = agent.icon
  return (
    <section className="border-b border-[var(--nova-border)] pb-4">
      <div className="flex items-center gap-3">
        <div className="flex h-9 w-9 items-center justify-center rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)]">
          <Icon className="h-4 w-4 text-[var(--nova-text-muted)]" />
        </div>
        <div className="min-w-0">
          <h1 className="truncate text-sm font-semibold">{t(agent.titleKey)}</h1>
          <div className="mt-1 text-[11px] text-[var(--nova-text-faint)]">{t(agent.subtitleKey)}</div>
        </div>
      </div>
    </section>
  )
}

function AgentModelSection({ value, inherited, profiles, onChange }: {
  value: AgentModelOverride
  inherited: AgentModelOverride
  profiles: Array<{ id: string; label: string }>
  onChange: (patch: Partial<AgentModelOverride>) => void
}) {
  const { t } = useTranslation()
  const hasProfile = hasTextOverride(value.profile_id)
  const hasTemperature = value.temperature !== undefined && value.temperature !== null
  const hasThinking = value.enable_thinking !== undefined && value.enable_thinking !== null
  const hasEffort = hasTextOverride(value.reasoning_effort)
  const effectiveProfile = hasProfile ? value.profile_id || 'default' : inherited.profile_id || 'default'
  const effectiveTemperature = hasTemperature ? value.temperature : inherited.temperature
  const effectiveThinking = hasThinking ? value.enable_thinking : inherited.enable_thinking
  const effectiveEffort = hasEffort ? value.reasoning_effort || '' : inherited.reasoning_effort || ''

  return (
    <section className="space-y-3 border-b border-[var(--nova-border)] pb-5">
      <SectionTitle icon={Brain} title={t('agents.section.model')} />
      <div className="grid gap-3 md:grid-cols-2">
        <Field label={t('agents.field.modelProfile')} inherited={!hasProfile} onReset={hasProfile ? () => onChange({ profile_id: '' }) : undefined}>
          <select value={effectiveProfile} onChange={(e) => onChange({ profile_id: e.target.value })} className={fieldCls}>
            {profiles.map((profile) => <option key={profile.id} value={profile.id}>{profile.label}</option>)}
          </select>
        </Field>
        <Field label="Temperature" inherited={!hasTemperature} onReset={hasTemperature ? () => onChange({ temperature: null }) : undefined}>
          <input
            type="number"
            step={0.1}
            min={0}
            max={2}
            value={effectiveTemperature ?? ''}
            placeholder={t('agents.option.platformDefault')}
            onChange={(e) => onChange({ temperature: e.target.value === '' ? null : Number(e.target.value) })}
            className={fieldCls}
          />
        </Field>
        <Field label={t('agents.field.thinking')}>
          <SwitchWithInheritance
            checked={thinkingDisplayValue(effectiveThinking)}
            onChange={(checked) => onChange({ enable_thinking: checked })}
            ariaLabel={t('agents.field.thinking')}
            statusLabel={thinkingStatusLabel(t, effectiveThinking)}
            inherited={!hasThinking}
            onReset={hasThinking ? () => onChange({ enable_thinking: null }) : undefined}
          />
        </Field>
        <Field label={t('agents.field.reasoningEffort')} inherited={!hasEffort} onReset={hasEffort ? () => onChange({ reasoning_effort: '' }) : undefined}>
          <select value={effectiveEffort} onChange={(e) => onChange({ reasoning_effort: e.target.value })} className={fieldCls}>
            <option value="">{t('agents.option.noSend')}</option>
            <option value="low">low</option>
            <option value="medium">medium</option>
            <option value="high">high</option>
          </select>
        </Field>
      </div>
    </section>
  )
}

function AgentPromptSection({ value, inherited, builtin, blocks, sources, onChange }: {
  value: AgentPromptOverride
  inherited: AgentPromptOverride
  builtin: string
  blocks?: AgentPromptBlocks
  sources?: AgentPromptSource[]
  onChange: (patch: Partial<AgentPromptOverride>) => void
}) {
  const { t } = useTranslation()
  const promptSources = sources?.length ? sources : fallbackPromptSources(blocks, builtin)
  return (
    <section className="space-y-3 border-b border-[var(--nova-border)] pb-5">
      <SectionTitle icon={ScrollText} title={t('agents.section.systemPrompt')} />
      <div className="space-y-2">
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
  const [open, setOpen] = useState(false)
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
                className={`${fieldCls} min-h-36 resize-y leading-5 shadow-none focus-visible:ring-0`}
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
      source: 'Nova runtime',
      content: blocks.runtime_contract,
    } : null,
    blocks?.output_protocol ? {
      id: 'output_protocol',
      title: 'Output Format',
      source: 'Nova runtime',
      content: blocks.output_protocol,
    } : null,
    {
      id: 'flow',
      title: 'Flow Rules',
      source: 'Nova built-in',
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

function AgentRuntimeContextSection({ agent, value, inherited, onChange }: {
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
  const effectiveCompactionEnabled = hasCompactionEnabled ? value.compaction_enabled : inherited.compaction_enabled ?? true
  const effectiveCompactionThreshold = hasCompactionThreshold ? value.compaction_threshold : inherited.compaction_threshold ?? 0.9
  const effectiveCompactionRecentTurns = hasCompactionRecentTurns ? value.compaction_recent_turns : inherited.compaction_recent_turns ?? 1
  const effectiveCompactionTargetMin = hasCompactionTargetMin ? value.compaction_target_min_ratio : inherited.compaction_target_min_ratio ?? 0.05
  const effectiveCompactionTargetMax = hasCompactionTargetMax ? value.compaction_target_max_ratio : inherited.compaction_target_max_ratio ?? 0.2
  const isCompactionAgent = agent === 'context_compaction'
  return (
    <section className="space-y-3 border-b border-[var(--nova-border)] pb-5">
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
              <input
                type="number"
                min={50}
                max={98}
                step={1}
                value={Math.round((effectiveCompactionThreshold ?? 0.9) * 100)}
                onChange={(e) => onChange({ compaction_threshold: e.target.value === '' ? null : Number(e.target.value) / 100 })}
                className={fieldCls}
              />
            </Field>
          </>
        )}
        {isCompactionAgent && (
          <>
            <Field label={t('agents.field.compactionRecentTurns')} inherited={!hasCompactionRecentTurns} onReset={hasCompactionRecentTurns ? () => onChange({ compaction_recent_turns: null }) : undefined}>
              <input
                type="number"
                min={1}
                max={30}
                step={1}
                value={effectiveCompactionRecentTurns ?? 1}
                onChange={(e) => onChange({ compaction_recent_turns: e.target.value === '' ? null : Number(e.target.value) })}
                className={fieldCls}
              />
            </Field>
            <Field label={t('agents.field.compactionTargetMin')} inherited={!hasCompactionTargetMin} onReset={hasCompactionTargetMin ? () => onChange({ compaction_target_min_ratio: null }) : undefined}>
              <input
                type="number"
                min={1}
                max={80}
                step={1}
                value={Math.round((effectiveCompactionTargetMin ?? 0.05) * 100)}
                onChange={(e) => onChange({ compaction_target_min_ratio: e.target.value === '' ? null : Number(e.target.value) / 100 })}
                className={fieldCls}
              />
            </Field>
            <Field label={t('agents.field.compactionTargetMax')} inherited={!hasCompactionTargetMax} onReset={hasCompactionTargetMax ? () => onChange({ compaction_target_max_ratio: null }) : undefined}>
              <input
                type="number"
                min={1}
                max={80}
                step={1}
                value={Math.round((effectiveCompactionTargetMax ?? 0.2) * 100)}
                onChange={(e) => onChange({ compaction_target_max_ratio: e.target.value === '' ? null : Number(e.target.value) / 100 })}
                className={fieldCls}
              />
            </Field>
          </>
        )}
      </div>
      <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-2 text-[11px] leading-5 text-[var(--nova-text-faint)]">
        {isCompactionAgent ? t('agents.context.compactionTargetNote') : t('agents.context.compactionNote')}
      </div>
    </section>
  )
}

function promptSourceTitle(t: ReturnType<typeof useTranslation>['t'], source: AgentPromptSource) {
  const key = `agents.prompt.source.${source.id}`
  const translated = t(key)
  return translated === key ? source.title : translated
}

function AgentToolSection({ agent, value, effective, onChange }: {
  agent: VisibleAgentKey
  value: AgentToolOverride
  effective: Required<AgentToolOverride>
  onChange: (key: ToolKey, value: boolean | null) => void
}) {
  const { t } = useTranslation()
  const rows = toolRowsForAgent(agent)
  return (
    <section className="space-y-3 border-b border-[var(--nova-border)] pb-5">
      <SectionTitle icon={Wrench} title={t('agents.section.tools')} />
      <div className="grid gap-2 lg:grid-cols-2">
        {rows.map((tool) => {
          const Icon = tool.icon
          const explicit = value[tool.key]
          const inherited = explicit === undefined || explicit === null
          const current = inherited ? effective[tool.key] : explicit
          return (
            <div key={tool.key} className="flex min-h-16 min-w-0 flex-col items-stretch gap-3 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 py-2 sm:flex-row sm:items-center">
              <Icon className="h-4 w-4 shrink-0 text-[var(--nova-text-muted)]" />
              <div className="min-w-0 flex-1">
                <div className="truncate font-medium">{t(tool.titleKey)}</div>
                <div className="mt-0.5 truncate text-[11px] text-[var(--nova-text-faint)]">
                  {t(toolSubtitleKey(tool, agent))}
                </div>
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
      </div>
    </section>
  )
}

function toolSubtitleKey(tool: AgentToolDefinition, agent: VisibleAgentKey) {
  if (agent === 'interactive_story' && tool.key === 'lore_read') {
    return 'agents.tool.loreRead.interactiveSubtitle'
  }
  return tool.subtitleKey
}

function toolRowsForAgent(agent: VisibleAgentKey) {
  if (agent === 'config_manager') return TOOL_ROWS
  return TOOL_ROWS.filter((tool) => tool.key !== 'agent_config_read' && tool.key !== 'agent_config_write')
}

function AgentSubAgentSection({ agent, inheritedModel, generalSettings, effectiveGeneralSettings, subAgents, effectiveSubAgents, profiles, onGeneralChange, onChange }: {
  agent: DeepAgentParentKey
  inheritedModel: AgentModelOverride
  generalSettings: Settings['general_sub_agents']
  effectiveGeneralSettings: Settings['general_sub_agents']
  subAgents: SubAgentConfig[]
  effectiveSubAgents: SubAgentConfig[]
  profiles: Array<{ id: string; label: string }>
  onGeneralChange: (agent: DeepAgentParentKey, value: boolean | null) => void
  onChange: (updater: (current: SubAgentConfig[]) => SubAgentConfig[]) => void
}) {
  const { t } = useTranslation()
  const [deleteTarget, setDeleteTarget] = useState<SubAgentConfig | null>(null)
  const [editingSubAgent, setEditingSubAgent] = useState<{ id: string; value: SubAgentConfig } | null>(null)
  const visibleSubAgents = useMemo(() => mergeVisibleSubAgents(effectiveSubAgents, subAgents)
    .filter((subAgent) => effectiveSubAgentParents(subAgent).includes(agent)), [agent, effectiveSubAgents, subAgents])
  const generalExplicit = generalSettings?.[agent]
  const generalEnabled = resolveGeneralSubAgentEnabled(effectiveGeneralSettings, agent)
  const addSubAgent = () => {
    const nextID = nextSubAgentID(mergeVisibleSubAgents(effectiveSubAgents, subAgents))
    setEditingSubAgent({
      id: nextID,
      value: {
        id: nextID,
        name: t('agents.subAgents.newName'),
        description: t('agents.subAgents.newDescription'),
        system_prompt: t('agents.subAgents.newPrompt'),
        enabled: true,
        parents: [agent],
        model: {},
        tools: {},
      },
    })
  }
  const updateSubAgent = (id: string, patch: Partial<SubAgentConfig>) => {
    const base = visibleSubAgents.find((subAgent) => normalizeSubAgentID(subAgent.id || '') === id)
    if (!base) return
    onChange((current) => upsertSubAgentOverride(current, normalizeSubAgentConfig({ ...base, ...patch }), id))
  }
  const setSubAgentAvailableForCurrent = (subAgent: SubAgentConfig, available: boolean) => {
    const id = normalizeSubAgentID(subAgent.id || '')
    if (!id) return
    const currentParents = effectiveSubAgentParents(subAgent)
    const nextParents = available
      ? DEEP_AGENT_PARENT_KEYS.filter((parent) => parent === agent || currentParents.includes(parent))
      : currentParents.filter((parent) => parent !== agent)
    updateSubAgent(id, { parents: nextParents, enabled: true })
  }
  const editSubAgent = (subAgent: SubAgentConfig) => {
    const id = normalizeSubAgentID(subAgent.id || '')
    if (!id) return
    setEditingSubAgent({ id, value: normalizeSubAgentConfig(subAgent) })
  }
  const updateEditingSubAgent = (id: string, patch: Partial<SubAgentConfig>) => {
    setEditingSubAgent((current) => {
      if (!current || current.id !== id) return current
      return { ...current, value: normalizeSubAgentConfig({ ...current.value, ...patch }) }
    })
  }
  const finishEditingSubAgent = () => {
    if (!editingSubAgent) return
    const next = normalizeSubAgentConfig(editingSubAgent.value)
    onChange((current) => upsertSubAgentOverride(current, next, editingSubAgent.id))
    setEditingSubAgent(null)
  }
  const deleteSubAgentForCurrentParent = () => {
    if (!deleteTarget) return
    const deleteID = normalizeSubAgentID(deleteTarget.id || '')
    if (!deleteID) return
    const base = visibleSubAgents.find((subAgent) => normalizeSubAgentID(subAgent.id || '') === deleteID) ?? deleteTarget
    onChange((current) => {
      return upsertSubAgentOverride(current, normalizeSubAgentConfig({ ...base, enabled: true, parents: subAgentParentsWithout(base, agent) }), deleteID)
    })
    if (editingSubAgent?.id === deleteID) setEditingSubAgent(null)
    setDeleteTarget(null)
  }
  const deleteSubAgentEverywhere = () => {
    if (!deleteTarget) return
    const deleteID = normalizeSubAgentID(deleteTarget.id || '')
    if (!deleteID) return
    const base = visibleSubAgents.find((subAgent) => normalizeSubAgentID(subAgent.id || '') === deleteID) ?? deleteTarget
    onChange((current) => {
      const currentHasID = current.some((subAgent) => normalizeSubAgentID(subAgent.id || '') === deleteID)
      if (!currentHasID) {
        return upsertSubAgentOverride(current, normalizeSubAgentConfig({ ...base, enabled: false, parents: [] }), deleteID)
      }
      return current.filter((subAgent) => normalizeSubAgentID(subAgent.id || '') !== deleteID)
    })
    if (editingSubAgent?.id === deleteID) setEditingSubAgent(null)
    setDeleteTarget(null)
  }

  return (
    <section className="space-y-3 border-b border-[var(--nova-border)] pb-5">
      <div className="flex min-w-0 flex-wrap items-center gap-2">
        <SectionTitle icon={Bot} title={t('agents.section.subAgents')} />
        <button
          type="button"
          onClick={addSubAgent}
          className="nova-nav-item ml-auto inline-flex items-center gap-1.5 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2.5 py-1 text-[11px] text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]"
        >
          <Plus className="h-3.5 w-3.5" />
          {t('agents.subAgents.add')}
        </button>
      </div>
      <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 py-2">
        <div className="flex min-w-0 items-center gap-2">
          <Bot className="mt-0.5 h-4 w-4 shrink-0 text-[var(--nova-text-muted)]" />
          <div className="min-w-0 flex-1">
            <div className="flex min-w-0 flex-wrap items-center gap-2">
              <span className="font-medium">{t('agents.subAgents.general.title')}</span>
              <span className={`rounded-[var(--nova-radius)] border px-1.5 py-0.5 text-[10px] ${generalEnabled ? 'border-[var(--nova-success)]/30 bg-[var(--nova-success-bg)] text-[var(--nova-success)]' : 'border-[var(--nova-danger)]/30 bg-[var(--nova-danger-bg)] text-[var(--nova-danger)]'}`}>
                {generalEnabled ? t('agents.option.on') : t('agents.option.off')}
              </span>
            </div>
            <div className="mt-1 text-[11px] leading-5 text-[var(--nova-text-faint)]">{t('agents.subAgents.general.description')}</div>
          </div>
          <SwitchWithInheritance
            checked={Boolean(generalEnabled)}
            onChange={(checked) => onGeneralChange(agent, checked)}
            ariaLabel={t('agents.subAgents.general.enabled')}
            inherited={generalExplicit === undefined || generalExplicit === null}
            onReset={generalExplicit !== undefined && generalExplicit !== null ? () => onGeneralChange(agent, null) : undefined}
          />
        </div>
      </div>
      {visibleSubAgents.length === 0 ? (
        <div className="rounded-[var(--nova-radius)] border border-dashed border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-3 text-[11px] text-[var(--nova-text-faint)]">
          {t('agents.subAgents.empty')}
        </div>
      ) : (
        <div className="space-y-2">
          {visibleSubAgents.map((subAgent, index) => (
            <SubAgentRow
              key={`${subAgent.id || 'subagent'}:${index}`}
              agent={agent}
              subAgent={subAgent}
              onToggle={(enabled) => setSubAgentAvailableForCurrent(subAgent, enabled)}
              onEdit={() => editSubAgent(subAgent)}
              onDelete={() => setDeleteTarget(subAgent)}
            />
          ))}
        </div>
      )}
      <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-2 text-[11px] leading-5 text-[var(--nova-text-faint)]">
        {t('agents.subAgents.note')}
      </div>
      <Dialog open={Boolean(editingSubAgent)} onOpenChange={(open) => { if (!open) setEditingSubAgent(null) }}>
        {editingSubAgent && (
          <DialogContent
            className="nova-panel flex max-h-[min(760px,calc(100vh-2rem))] flex-col overflow-hidden rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] p-0 text-[var(--nova-text)] shadow-[var(--nova-shadow)]"
          >
            <DialogHeader className="shrink-0 gap-1 border-b border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-4 py-3 text-left">
              <DialogTitle className="text-sm">{editingSubAgent.value.name || editingSubAgent.value.id || t('agents.subAgents.untitled')}</DialogTitle>
              <DialogDescription className="text-[11px] leading-5 text-[var(--nova-text-faint)]">
                {t('agents.subAgents.dialogDescription')}
              </DialogDescription>
            </DialogHeader>
            <div className="min-h-0 flex-1 overflow-y-auto px-4 py-4">
              <SubAgentEditor
                id={editingSubAgent.id}
                agent={agent}
                subAgent={editingSubAgent.value}
                inheritedModel={inheritedModel}
                profiles={profiles}
                onChange={updateEditingSubAgent}
              />
            </div>
            <DialogFooter className="mx-0 mb-0 shrink-0 border-t border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-4 py-3">
              <button type="button" onClick={finishEditingSubAgent} className="nova-nav-item rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 py-1.5 text-xs text-[var(--nova-text)] hover:bg-[var(--nova-hover)]">
                {t('agents.subAgents.done')}
              </button>
            </DialogFooter>
          </DialogContent>
        )}
      </Dialog>
      <AlertDialog open={deleteTarget !== null} onOpenChange={(open) => { if (!open) setDeleteTarget(null) }}>
        <AlertDialogContent size="sm" className="border-[var(--nova-border)] bg-[var(--nova-surface)] text-[var(--nova-text)] shadow-[var(--nova-shadow)]">
          <AlertDialogHeader>
            <AlertDialogTitle>{t('agents.subAgents.deleteTitle')}</AlertDialogTitle>
            <AlertDialogDescription>{t('agents.subAgents.deleteDescription')}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('common.cancel')}</AlertDialogCancel>
            <button type="button" onClick={deleteSubAgentForCurrentParent} className="nova-nav-item rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-1.5 text-xs text-[var(--nova-text)] hover:bg-[var(--nova-hover)]">
              {t('agents.subAgents.deleteCurrentParent')}
            </button>
            <AlertDialogAction variant="destructive" onClick={deleteSubAgentEverywhere}>{t('agents.subAgents.deleteEverywhere')}</AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </section>
  )
}

function SubAgentRow({ agent, subAgent, onToggle, onEdit, onDelete }: {
  agent: DeepAgentParentKey
  subAgent: SubAgentConfig
  onToggle: (enabled: boolean) => void
  onEdit: () => void
  onDelete: () => void
}) {
  const { t } = useTranslation()
  const parents = effectiveSubAgentParents(subAgent)
  const availableForCurrent = parents.includes(agent)
  const enabled = (subAgent.enabled ?? true) && availableForCurrent
  return (
    <div className="flex min-w-0 items-center gap-2 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 py-2">
      <Bot className="h-4 w-4 shrink-0 text-[var(--nova-text-muted)]" />
      <div className="min-w-0 flex-1">
        <div className="flex min-w-0 items-center gap-2">
          <span className="min-w-0 truncate font-medium">{subAgent.name || subAgent.id || t('agents.subAgents.untitled')}</span>
          <span className={`shrink-0 rounded-[var(--nova-radius)] border px-1.5 py-0.5 text-[10px] ${enabled ? 'border-[var(--nova-success)]/30 bg-[var(--nova-success-bg)] text-[var(--nova-success)]' : 'border-[var(--nova-danger)]/30 bg-[var(--nova-danger-bg)] text-[var(--nova-danger)]'}`}>
            {enabled ? t('agents.option.on') : t('agents.option.off')}
          </span>
          {!availableForCurrent && (
            <span className="shrink-0 rounded bg-[var(--nova-danger-bg)] px-1.5 py-0.5 text-[10px] text-[var(--nova-danger)]">{t('agents.subAgents.unavailableShort')}</span>
          )}
        </div>
        <div className="mt-1 flex min-w-0 flex-wrap gap-x-2 gap-y-1 text-[11px] text-[var(--nova-text-faint)]">
          <span className="font-mono">{subAgent.id}</span>
          <span>{parents.map((parent) => t(`agents.subAgents.parent.${parent}`)).join(', ')}</span>
          <span>{subAgentToolSummary(t, subAgent.tools)}</span>
        </div>
      </div>
      <ToggleSwitch
        checked={enabled}
        onChange={onToggle}
        ariaLabel={t('agents.subAgents.enabled')}
      />
      <button type="button" onClick={onEdit} className="nova-icon-button flex h-7 w-7 shrink-0 items-center justify-center rounded-[var(--nova-radius)] border border-[var(--nova-border)] text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]" aria-label={t('agents.subAgents.edit')}>
        <Edit3 className="h-3.5 w-3.5" />
      </button>
      <button type="button" onClick={onDelete} className="nova-icon-button flex h-7 w-7 shrink-0 items-center justify-center rounded-[var(--nova-radius)] border border-[var(--nova-border)] text-[var(--nova-text-muted)] hover:text-[var(--nova-danger)]" aria-label={t('agents.subAgents.delete')}>
        <Trash2 className="h-3.5 w-3.5" />
      </button>
    </div>
  )
}

function SubAgentEditor({ id, agent, subAgent, inheritedModel, profiles, onChange }: {
  id: string
  agent: DeepAgentParentKey
  subAgent: SubAgentConfig
  inheritedModel: AgentModelOverride
  profiles: Array<{ id: string; label: string }>
  onChange: (id: string, patch: Partial<SubAgentConfig>) => void
}) {
  const { t } = useTranslation()
  const parents = effectiveSubAgentParents(subAgent)
  const parentSet = new Set(parents)
  const tools = subAgent.tools ?? {}
  const rows = toolRowsForAgent(agent)
  const model = subAgent.model ?? {}
  const hasThinking = model.enable_thinking !== undefined && model.enable_thinking !== null
  const effectiveThinking = hasThinking ? model.enable_thinking : inheritedModel.enable_thinking

  const setModel = (patch: Partial<AgentModelOverride>) => onChange(id, { model: { ...model, ...patch } })
  const setTool = (key: ToolKey, value: boolean | null) => {
    const nextTools = { ...tools, [key]: value }
    if (value === null) delete nextTools[key]
    onChange(id, { tools: nextTools })
  }
  const setParent = (parent: DeepAgentParentKey, checked: boolean) => {
    const current = effectiveSubAgentParents(subAgent)
    const next = new Set(current)
    if (checked) next.add(parent)
    else next.delete(parent)
    const ordered = DEEP_AGENT_PARENT_KEYS.filter((key) => next.has(key))
    onChange(id, { parents: ordered })
  }

  return (
    <div className="space-y-3">
      <div className="grid gap-3 md:grid-cols-2">
        <Field label={t('agents.subAgents.id')}>
          <input value={subAgent.id ?? ''} onChange={(e) => onChange(id, { id: normalizeSubAgentID(e.target.value) })} className={fieldCls} />
        </Field>
        <Field label={t('agents.subAgents.name')}>
          <input value={subAgent.name ?? ''} onChange={(e) => onChange(id, { name: e.target.value })} className={fieldCls} />
        </Field>
      </div>
      <div className="grid gap-3 md:grid-cols-2">
        <Field label={t('agents.subAgents.description')}>
          <input value={subAgent.description ?? ''} onChange={(e) => onChange(id, { description: e.target.value })} className={fieldCls} />
        </Field>
        <Field label={t('agents.field.modelProfile')}>
          <select value={model.profile_id || ''} onChange={(e) => setModel({ profile_id: e.target.value })} className={fieldCls}>
            <option value="">{t('agents.option.inherit')}</option>
            {profiles.map((profile) => <option key={profile.id} value={profile.id}>{profile.label}</option>)}
          </select>
        </Field>
      </div>
      <Field label={t('agents.subAgents.prompt')}>
        <Textarea
          autoResize
          value={subAgent.system_prompt ?? ''}
          onChange={(e) => onChange(id, { system_prompt: e.target.value })}
          placeholder={t('agents.subAgents.promptPlaceholder')}
          className={`${fieldCls} min-h-28 resize-y leading-5 shadow-none focus-visible:ring-0`}
        />
      </Field>
      <div className="grid gap-3 md:grid-cols-3">
        <Field label="Temperature">
          <input
            type="number"
            min={0}
            max={2}
            step={0.1}
            value={model.temperature ?? ''}
            placeholder={t('agents.option.inherit')}
            onChange={(e) => setModel({ temperature: e.target.value === '' ? null : Number(e.target.value) })}
            className={fieldCls}
          />
        </Field>
        <Field label={t('agents.field.thinking')}>
          <SwitchWithInheritance
            checked={thinkingDisplayValue(effectiveThinking)}
            onChange={(checked) => setModel({ enable_thinking: checked })}
            ariaLabel={t('agents.field.thinking')}
            statusLabel={thinkingStatusLabel(t, effectiveThinking)}
            inherited={!hasThinking}
            onReset={hasThinking ? () => setModel({ enable_thinking: null }) : undefined}
          />
        </Field>
        <Field label={t('agents.field.reasoningEffort')}>
          <select value={model.reasoning_effort || ''} onChange={(e) => setModel({ reasoning_effort: e.target.value })} className={fieldCls}>
            <option value="">{t('agents.option.inherit')}</option>
            <option value="low">low</option>
            <option value="medium">medium</option>
            <option value="high">high</option>
          </select>
        </Field>
      </div>
      <div>
        <div className="mb-1.5 text-[var(--nova-text-muted)]">{t('agents.subAgents.parents')}</div>
        <div className="flex flex-wrap gap-2">
          {DEEP_AGENT_PARENT_KEYS.map((parent) => (
            <label key={parent} className="inline-flex items-center gap-1.5 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 py-1 text-[11px] text-[var(--nova-text-muted)]">
              <input type="checkbox" checked={parentSet.has(parent)} onChange={(e) => setParent(parent, e.target.checked)} />
              {t(`agents.subAgents.parent.${parent}`)}
            </label>
          ))}
        </div>
        {!parentSet.has(agent) && (
          <div className="mt-1.5 text-[11px] text-[var(--nova-danger)]">{t('agents.subAgents.notAvailableForCurrent')}</div>
        )}
      </div>
      <div>
        <div className="mb-1.5 text-[var(--nova-text-muted)]">{t('agents.subAgents.tools')}</div>
        <div className="grid gap-2 md:grid-cols-2">
          {rows.map((tool) => (
            <div key={tool.key} className="flex min-w-0 items-center gap-2 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 py-1.5">
              <span className="min-w-0 flex-1 truncate text-[11px]">{t(tool.titleKey)}</span>
              <SwitchWithInheritance
                checked={tools[tool.key] ?? true}
                onChange={(checked) => setTool(tool.key, checked)}
                ariaLabel={t(tool.titleKey)}
                inherited={tools[tool.key] === undefined || tools[tool.key] === null}
                onReset={tools[tool.key] !== undefined && tools[tool.key] !== null ? () => setTool(tool.key, null) : undefined}
              />
            </div>
          ))}
        </div>
      </div>
    </div>
  )
}

function AgentSkillSection({ agent, skills, value, effective, onChange }: {
  agent: VisibleAgentKey
  skills: SkillSummary[]
  value: AgentSkillOverride
  effective: Settings['agent_skills']
  onChange: (name: string, value: boolean | null) => void
}) {
  const { t } = useTranslation()
  return (
    <section className="space-y-3 border-b border-[var(--nova-border)] pb-5">
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
                  <div className="mt-0.5 truncate text-[11px] text-[var(--nova-text-faint)]" title={skill.description}>{skill.description}</div>
                  <div className="mt-0.5 truncate text-[10px] text-[var(--nova-text-faint)]">
                    {defaultAvailable ? t('agents.skills.defaultAvailable') : t('agents.skills.defaultUnavailable')}
                    {skill.agent ? ` · ${skill.agent}` : ''}
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

function AgentBuiltInCapabilitySection({ agent }: { agent: VisibleAgentKey }) {
  const { t } = useTranslation()
  const rows = builtInCapabilityRows(agent, t)
  return (
    <section className="space-y-3 border-b border-[var(--nova-border)] pb-5">
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

function AgentModelOnlySection() {
  const { t } = useTranslation()
  return (
    <section className="space-y-3 border-b border-[var(--nova-border)] pb-5">
      <SectionTitle icon={Wrench} title={t('agents.section.tools')} />
      <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 py-2 text-[11px] leading-5 text-[var(--nova-text-faint)]">
        {t('agents.modelOnly.note')}
      </div>
    </section>
  )
}

function AgentContextSection({ agent, effective }: { agent: VisibleAgentKey; effective: Settings }) {
  const { t } = useTranslation()
  const rows = contextRowsFor(agent, effective, t)
  return (
    <section className="space-y-3 pb-5">
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

function SectionTitle({ icon: Icon, title }: { icon: ElementType; title: string }) {
  return (
    <div className="flex items-center gap-2 text-xs font-medium">
      <Icon className="h-3.5 w-3.5 text-[var(--nova-text-muted)]" />
      {title}
    </div>
  )
}

function Field({ label, inherited, onReset, children }: { label: string; inherited?: boolean; onReset?: () => void; children: ReactNode }) {
  return (
    <div className="flex min-w-0 flex-col gap-1.5">
      <span className="text-[var(--nova-text-muted)]">{label}</span>
      <span className="flex min-w-0 flex-wrap items-center gap-2">
        {children}
        {inherited !== undefined && <InheritanceBadge inherited={inherited} onReset={onReset} />}
      </span>
    </div>
  )
}

function ToggleSwitch({ checked, onChange, ariaLabel, statusLabel }: { checked: boolean; onChange: (checked: boolean) => void; ariaLabel: string; statusLabel?: string }) {
  const { t } = useTranslation()
  const label = statusLabel || (checked ? t('agents.option.on') : t('agents.option.off'))
  const dotClass = statusDotClass(label, checked, t)
  return (
    <span className="inline-flex shrink-0 items-center gap-1.5" title={`${ariaLabel}: ${label}`}>
      <Switch
        checked={checked}
        onCheckedChange={onChange}
        aria-label={ariaLabel}
        title={`${ariaLabel}: ${label}`}
      />
      <span aria-hidden="true" className={`size-1.5 shrink-0 rounded-full ${dotClass}`} />
    </span>
  )
}

function SwitchWithInheritance({ checked, onChange, ariaLabel, statusLabel, inherited, onReset }: {
  checked: boolean
  onChange: (checked: boolean) => void
  ariaLabel: string
  statusLabel?: string
  inherited: boolean
  onReset?: () => void
}) {
  return (
    <span className="inline-flex shrink-0 items-center gap-1.5">
      <ToggleSwitch checked={checked} onChange={onChange} ariaLabel={ariaLabel} statusLabel={statusLabel} />
      <InheritanceText inherited={inherited} onReset={onReset} />
    </span>
  )
}

function InheritanceText({ inherited, onReset }: { inherited: boolean; onReset?: () => void }) {
  const { t } = useTranslation()
  if (inherited) {
    return <span className="w-7 text-center text-[10px] leading-none text-[var(--nova-text-faint)]">{t('agents.badge.inherited')}</span>
  }
  return (
    <button type="button" onClick={onReset} className="w-7 text-center text-[10px] leading-none text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]">
      {t('agents.badge.overridden')}
    </button>
  )
}

function InheritanceBadge({ inherited, onReset }: { inherited: boolean; onReset?: () => void }) {
  const { t } = useTranslation()
  return (
    <span className={`inline-flex h-7 max-w-full shrink-0 items-center rounded-[var(--nova-radius)] border px-2 text-[11px] ${inherited ? 'border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-faint)]' : 'border-[var(--nova-border)] bg-[var(--nova-active)] text-[var(--nova-text-muted)]'}`}>
      {inherited ? t('agents.badge.inherited') : (
        <button type="button" onClick={onReset} className="text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]">
          {t('agents.badge.overridden')}
        </button>
      )}
    </span>
  )
}

function buildProfileOptions(draft: Settings, effective: Settings, t: (key: string, options?: Record<string, unknown>) => string): Array<{ id: string; label: string }> {
  const profiles = new Map<string, string>()
  const add = (profile?: ModelProfileSettings) => {
    const id = modelProfileID(profile)
    if (!id) return
    profiles.set(id, modelProfileLabel(profile))
  }
  modelProfilesWithDefault(effective).forEach(add)
  ;(draft.model_profiles ?? []).forEach(add)
  if (!profiles.has('default')) profiles.set('default', t('agents.option.defaultModel'))
  return Array.from(profiles.entries()).map(([id, label]) => ({
    id,
    label: id === 'default' ? t('agents.option.defaultProfile', { label }) : t('agents.option.profile', { id, label }),
  }))
}

function contextRowsFor(agent: VisibleAgentKey, effective: Settings, t: (key: string, options?: Record<string, unknown>) => string) {
  const context = mergeAgentContextOverride(effective.agent_context?.default ?? {}, effective.agent_context?.[agent] ?? {})
  const compactionContext = mergeAgentContextOverride(effective.agent_context?.default ?? {}, effective.agent_context?.context_compaction ?? {})
  const compactionTurns = compactionContext.compaction_recent_turns ?? 1
  const threshold = Math.round((context.compaction_threshold ?? 0.9) * 100)
  const targetMin = Math.round((compactionContext.compaction_target_min_ratio ?? 0.05) * 100)
  const targetMax = Math.round((compactionContext.compaction_target_max_ratio ?? 0.2) * 100)
  if (agent === 'ide') {
    return [
      { title: t('agents.context.currentBook'), value: 'workspace' },
      { title: t('agents.context.defaultTeller'), value: effective.ide_story_teller_id || 'classic' },
      { title: t('agents.context.sessionContext'), value: t('agents.context.compactionValue', { threshold }) },
    ]
  }
  if (agent === 'interactive_story') {
    return [
      { title: t('agents.context.storyState'), value: 'story jsonl' },
      { title: t('agents.context.teller'), value: t('agents.context.currentStoryTeller') },
      { title: t('agents.context.sessionContext'), value: t('agents.context.compactionValue', { threshold }) },
    ]
  }
  if (agent === 'context_compaction') {
    return [
      { title: t('agents.context.inputSource'), value: t('agents.context.compactionInputValue') },
      { title: t('agents.context.outputShape'), value: t('agents.context.compactionOutputValue') },
      { title: t('agents.context.historyBoundary'), value: t('agents.context.compactionTargetValue', { count: compactionTurns, min: targetMin, max: targetMax }) },
    ]
  }
  return [
    { title: t('agents.context.inputSource'), value: t('agents.context.inputSourceValue') },
    { title: t('agents.context.outputShape'), value: t('agents.context.outputShapeValue') },
    { title: t('agents.context.historyBoundary'), value: t('agents.context.compactionValue', { threshold }) },
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

function thinkingDisplayValue(value?: boolean | null) {
  return value ?? true
}

function thinkingStatusLabel(t: (key: string) => string, value?: boolean | null) {
  if (value === undefined || value === null) return t('agents.option.default')
  return value ? t('agents.option.on') : t('agents.option.off')
}

function statusDotClass(label: string, checked: boolean, t: (key: string) => string) {
  if (label === t('agents.option.default')) return 'bg-[var(--nova-text-faint)]'
  return checked ? 'bg-[var(--nova-success)]' : 'bg-[var(--nova-danger)]'
}

function hasPromptOverride(value?: string) {
  return value !== undefined && value.trim() !== ''
}

function isDeepAgentParent(agent: VisibleAgentKey): agent is DeepAgentParentKey {
  return (DEEP_AGENT_PARENT_KEYS as string[]).includes(agent)
}

const GENERAL_SUB_AGENT_KEYS = ['default', 'ide', 'interactive_story', 'config_manager', 'automation'] as const

function defaultGeneralSubAgentSettings(): Settings['general_sub_agents'] {
  return { default: false, ide: true, automation: true }
}

function previewGeneralSubAgentSettings(layered: LayeredSettings | null, activeLayer: SettingsLayer, draft: Settings): Settings['general_sub_agents'] {
  let settings = defaultGeneralSubAgentSettings()
  if (!layered) return mergeGeneralSubAgentSettings(settings, draft.general_sub_agents)
  settings = mergeGeneralSubAgentSettings(settings, layered.default.general_sub_agents)
  settings = mergeGeneralSubAgentSettings(settings, layered.global.general_sub_agents)
  settings = mergeGeneralSubAgentSettings(settings, activeLayer === 'user' ? draft.general_sub_agents : layered.user.general_sub_agents)
  settings = mergeGeneralSubAgentSettings(settings, activeLayer === 'workspace' ? draft.general_sub_agents : layered.workspace.general_sub_agents)
  return settings
}

function mergeGeneralSubAgentSettings(parent: Settings['general_sub_agents'], child: Settings['general_sub_agents']): Settings['general_sub_agents'] {
  const out: Settings['general_sub_agents'] = { ...(parent ?? {}) }
  if (!child) return out
  for (const key of GENERAL_SUB_AGENT_KEYS) {
    const value = child[key]
    if (value !== undefined && value !== null) out[key] = value
  }
  return out
}

function resolveGeneralSubAgentEnabled(settings: Settings['general_sub_agents'], agent: DeepAgentParentKey) {
  const fallback = settings?.default ?? false
  return settings?.[agent] ?? fallback
}

function subAgentToolSummary(t: (key: string, options?: Record<string, unknown>) => string, tools?: AgentToolOverride) {
  const overrides = TOOL_ROWS.filter((tool) => tools?.[tool.key] !== undefined && tools?.[tool.key] !== null)
  if (overrides.length === 0) return t('agents.subAgents.toolsInherited')
  return t('agents.subAgents.toolsRestricted', { count: overrides.length })
}

function nextSubAgentID(current: SubAgentConfig[]) {
  const used = new Set(current.map((subAgent) => subAgent.id).filter(Boolean))
  for (let index = 1; index < 1000; index += 1) {
    const id = `subagent-${index}`
    if (!used.has(id)) return id
  }
  return `subagent-${Date.now()}`
}

function normalizeSubAgentID(value: string) {
  return value
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9_-]+/g, '-')
    .replace(/[-_]{2,}/g, '-')
    .replace(/^[-_]+|[-_]+$/g, '')
}

function normalizeSubAgentConfig(value: SubAgentConfig): SubAgentConfig {
  return {
    ...value,
    id: normalizeSubAgentID(value.id || ''),
    name: value.name ?? '',
    description: value.description ?? '',
    system_prompt: value.system_prompt ?? '',
    parents: sanitizeSubAgentParents(value.parents),
  }
}

function mergeVisibleSubAgents(effective: SubAgentConfig[], draft: SubAgentConfig[]) {
  const rows: SubAgentConfig[] = []
  const index = new Map<string, number>()
  for (const subAgent of effective) {
    const id = normalizeSubAgentID(subAgent.id || '')
    if (!id || index.has(id)) continue
    index.set(id, rows.length)
    rows.push(normalizeSubAgentConfig(subAgent))
  }
  for (const subAgent of draft) {
    const normalized = normalizeSubAgentConfig(subAgent)
    const id = normalizeSubAgentID(normalized.id || '')
    if (!id) continue
    const existing = index.get(id)
    if (existing === undefined) {
      index.set(id, rows.length)
      rows.push(normalized)
    } else {
      rows[existing] = normalized
    }
  }
  return rows
}

function upsertSubAgentOverride(current: SubAgentConfig[], next: SubAgentConfig, previousID?: string) {
  const nextID = normalizeSubAgentID(next.id || '')
  const oldID = normalizeSubAgentID(previousID || nextID)
  if (!nextID) return current
  const filtered = current.filter((subAgent) => {
    const id = normalizeSubAgentID(subAgent.id || '')
    return id !== nextID && id !== oldID
  })
  return [...filtered, next]
}

function sanitizeSubAgentParents(value?: string[]) {
  if (!value || value.length === 0) return []
  const selected = DEEP_AGENT_PARENT_KEYS.filter((parent) => value.includes(parent))
  return selected
}

function effectiveSubAgentParents(subAgent: SubAgentConfig): DeepAgentParentKey[] {
  const parents = sanitizeSubAgentParents(subAgent.parents)
  return parents as DeepAgentParentKey[]
}

function subAgentParentsWithout(subAgent: SubAgentConfig, agent: DeepAgentParentKey): DeepAgentParentKey[] {
  return effectiveSubAgentParents(subAgent).filter((parent) => parent !== agent)
}

function mergeAgentModelOverride(parent: AgentModelOverride, child: AgentModelOverride): AgentModelOverride {
  return {
    profile_id: child.profile_id || parent.profile_id,
    temperature: child.temperature ?? parent.temperature,
    enable_thinking: child.enable_thinking ?? parent.enable_thinking,
    reasoning_effort: child.reasoning_effort || parent.reasoning_effort,
  }
}

function mergeAgentPromptOverride(parent: AgentPromptOverride, child: AgentPromptOverride): AgentPromptOverride {
  return {
    flow_prompt: hasPromptOverride(child.flow_prompt) ? child.flow_prompt : parent.flow_prompt,
    system_prompt: hasPromptOverride(child.system_prompt) ? child.system_prompt : parent.system_prompt,
  }
}

function mergeAgentContextOverride(parent: AgentContextOverride, child: AgentContextOverride): AgentContextOverride {
  const compactionThreshold = child.compaction_threshold ?? parent.compaction_threshold ?? 0.9
  const compactionRecentTurns = child.compaction_recent_turns ?? parent.compaction_recent_turns ?? 1
  const compactionTargetMin = child.compaction_target_min_ratio ?? parent.compaction_target_min_ratio ?? 0.05
  const compactionTargetMax = child.compaction_target_max_ratio ?? parent.compaction_target_max_ratio ?? 0.2
  return {
    compaction_enabled: child.compaction_enabled ?? parent.compaction_enabled ?? true,
    compaction_threshold: Math.max(0.5, Math.min(0.98, compactionThreshold)),
    compaction_recent_turns: Math.max(1, Math.min(30, compactionRecentTurns)),
    compaction_target_min_ratio: Math.max(0.01, Math.min(0.8, compactionTargetMin)),
    compaction_target_max_ratio: Math.max(0.01, Math.min(0.8, Math.max(compactionTargetMin, compactionTargetMax))),
  }
}
