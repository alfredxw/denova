import { useCallback, useEffect, useMemo, useState } from 'react'
import { Bot } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { ConfigManagerChat } from '@/components/Chat/ConfigManagerChat'
import { AutosaveStatusIndicator } from '@/components/forms/autosave-status'
import { AdaptiveSurface } from '@/components/layout/adaptive-surface'
import { FeaturePageShell } from '@/components/layout/feature-page-shell'
import { MobilePaneTrigger } from '@/components/layout/mobile-pane-trigger'
import { SectionedNavigation } from '@/components/navigation/sectioned-navigation'
import { Button } from '@/components/ui/button'
import type { AgentContextOverride, AgentModelOverride, AgentPromptOverride, AgentSkillOverride, ModelProfileSettings, Settings, SettingsLayer, SubAgentConfig } from '@/features/settings/types'
import { modelProfileID, modelProfileLabel, modelProfilesWithDefault } from '@/features/settings/model-profiles'
import { useLayeredSettingsDraft } from '@/features/settings/use-layered-settings-draft'
import { getSkills } from '@/lib/api'
import type { SkillSummary } from '@/lib/api'
import { AgentRuntimeContextSection } from './AgentRuntimeContextSection'
import { AgentBuiltInCapabilitySection, AgentContextSection, AgentModelOnlySection, AgentModelSection, AgentPromptSection, AgentSkillSection, AgentToolSection, mergeAgentContextOverride, mergeAgentModelOverride, mergeAgentPromptOverride } from './agent-configuration-sections'
import { AgentSubAgentSection, isSubAgentParent, previewGeneralSubAgentSettings } from './agent-subagent-section'
import { AGENTS, FALLBACK_AGENT_TOOL_VALUES, resolveEffectiveTools } from './agent-registry'
import type { AgentViewDefinition, SubAgentParentKey, ToolKey, VisibleAgentKey } from './agent-registry'

const tabCls = 'nova-nav-item rounded-[var(--nova-radius)] px-2.5 py-1 text-xs'

export function AgentsView({ onClose }: { onClose?: () => void }) {
  const { t } = useTranslation()
  const [activeLayer, setActiveLayer] = useState<SettingsLayer>('user')
  const { layered, draft, setDraft, error, autosaveStatus, autosaveError, reload, notifyUpdated, saveNow } = useLayeredSettingsDraft({
    layer: activeLayer,
    sourcePrefix: 'agents-view',
  })
  const [activeAgent, setActiveAgent] = useState<VisibleAgentKey>('ide')
  const [skills, setSkills] = useState<SkillSummary[]>([])
  const [agentChatOpen, setAgentChatOpen] = useState(false)

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

  const reloadAfterAgentMutation = useCallback(() => {
    void saveNow()
      .then(async () => {
        await reload()
        notifyUpdated()
      })
      .catch(() => undefined)
  }, [notifyUpdated, reload, saveNow])

  const switchLayer = async (layer: SettingsLayer) => {
    if (layer === activeLayer) return
    try {
      await saveNow()
      setActiveLayer(layer)
    } catch {
      // The layered settings hook already exposes the actionable save error.
    }
  }

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

  const setGeneralSubAgent = (agent: SubAgentParentKey, value: boolean | null) => {
    setDraft((current) => {
      const next = { ...(current.general_sub_agents ?? {}) }
      if (value === null) delete next[agent]
      else next[agent] = value
      return { ...current, general_sub_agents: next }
    })
  }

  return (
    <FeaturePageShell
      icon={Bot}
      title="Agents"
      className="bg-[var(--nova-bg)]"
      topbarClassName="max-md:flex-wrap max-md:overflow-x-hidden"
      error={error}
      errorTitle={t('agents.saveError')}
      onClose={onClose ? () => {
        void saveNow().then(() => onClose()).catch(() => undefined)
      } : undefined}
      closeLabel={t('agents.close')}
      onSaveShortcut={() => saveNow().catch(() => undefined)}
      headerContent={(
        <div className="flex shrink-0 gap-1 border-l border-[var(--nova-border)] pl-2 sm:ml-3 sm:pl-3">
          {(['user', 'workspace'] as SettingsLayer[]).map((layer) => (
            <Button
              key={layer}
              type="button"
              variant="ghost"
              size="xs"
              onClick={() => void switchLayer(layer)}
              className={`${tabCls} ${activeLayer === layer ? 'is-active' : 'bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)]'}`}
            >
              {layer === 'workspace' ? t('agents.layer.workspace') : t('agents.layer.user')}
            </Button>
          ))}
        </div>
      )}
      actions={(
        <>
          <AutosaveStatusIndicator
            status={autosaveStatus}
            error={autosaveError}
            onRetry={() => saveNow().catch(() => undefined)}
          />
          <Button
            type="button"
            onClick={() => setAgentChatOpen((value) => !value)}
            variant={agentChatOpen ? 'secondary' : 'outline'}
            size="sm"
            aria-pressed={agentChatOpen}
          >
            <Bot data-icon="inline-start" />
            {t('agents.configAgent.button')}
          </Button>
        </>
      )}
    >
      <AdaptiveSurface
        left={{
          id: 'agents-list',
          title: 'Agents',
          side: 'left',
          icon: <Bot className="h-4 w-4" />,
          content: <div className="h-full min-h-0 overflow-y-auto bg-[var(--nova-surface-2)] p-3"><AgentList active={activeAgent} onSelect={setActiveAgent} /></div>,
          desktopClassName: 'w-72 shrink-0 min-h-0 border-r border-[var(--nova-border)]',
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
        rightResize={{
          layoutKey: 'nova-agents-config-manager-layout',
          label: t('layout.resize.right'),
          defaultSize: '420px',
          minSize: '300px',
          maxSize: '65%',
          mainMinSize: '240px',
        }}
      >
        {({ openLeft, openRight }) => (
          <main className="h-full min-h-0 overflow-y-auto overflow-x-hidden">
            <div className="sticky top-0 z-10 flex h-10 items-center gap-2 border-b border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 md:hidden">
              <MobilePaneTrigger side="left" label={t('workbench.mobile.openSidePanel', { label: 'Agents' })} onClick={openLeft} />
              <span className="min-w-0 truncate text-[11px] text-[var(--nova-text-muted)]">{t(selected.titleKey)}</span>
              {agentChatOpen && (
                <MobilePaneTrigger side="right" label={t('workbench.mobile.openSidePanel', { label: t('agents.configAgent.title') })} onClick={openRight} className="ml-auto" />
              )}
            </div>
            <div className="mx-auto flex w-full min-w-0 max-w-5xl flex-col gap-5 px-4 py-5 sm:px-6">
              <AgentHeader agent={selected} />
              {activeLayer === 'user' ? (
                <AgentModelSection
                  value={modelValue}
                  inherited={inheritedModel}
                  profiles={profileOptions}
                  onChange={setAgentModel}
                />
              ) : (
                <section className="border-b border-[var(--nova-border)] pb-5 text-xs text-[var(--nova-text-muted)]">
                  {t('agents.model.userScoped')}
                </section>
              )}
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
                  {isSubAgentParent(activeAgent) && (
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
    </FeaturePageShell>
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
    <SectionedNavigation
      groups={groups.map((group, index) => ({
        id: `${group.group}:${index}`,
        title: t(group.group),
        items: group.agents.map((agent) => ({
          id: agent.key,
          title: t(agent.titleKey),
          description: t(agent.subtitleKey),
          icon: agent.icon,
        })),
      }))}
      activeId={active}
      onSelect={onSelect}
      itemClassName="py-2"
    />
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
