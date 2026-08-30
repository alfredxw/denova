import { useCallback, useEffect, useId, useMemo, useRef, useState } from 'react'
import { Bot } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { ConfigManagerChat } from '@/components/Chat/ConfigManagerChat'
import { ConfigManagerToggle } from '@/components/Chat/ConfigManagerToggle'
import { AutosaveStatusIndicator } from '@/components/forms/autosave-status'
import { SettingsFieldRow } from '@/components/forms/settings-field-row'
import { AdaptiveSurface } from '@/components/layout/adaptive-surface'
import { FeaturePageShell } from '@/components/layout/feature-page-shell'
import { MobilePaneTrigger } from '@/components/layout/mobile-pane-trigger'
import { SidebarVisibilityToggle } from '@/components/layout/sidebar-visibility-toggle'
import { Button } from '@/components/ui/button'
import { LoadingState } from '@/components/common/LoadingState'
import { Input } from '@/components/ui/input'
import type { AgentContextOverride, AgentModelOverride, AgentPromptOverride, AgentSkillOverride, AgentToolOverride, CustomAgentBaseKind, CustomAgentConfig, ImageAPIProfileSettings, LayeredSettings, ModelProfileSettings, Settings, SettingsLayer } from '@/features/settings/types'
import { modelProfileID, modelProfileLabel, modelProfilesWithDefault } from '@/features/settings/model-profiles'
import { imageAPIProfileID, imageAPIProfileLabel, imageAPIProfilesWithDefault } from '@/features/settings/image-profiles'
import { useLayeredSettingsDraft } from '@/features/settings/use-layered-settings-draft'
import { getSkills, resourceTargetKey } from '@/lib/api'
import type { ResourceTarget, SkillSummary } from '@/lib/api'
import { AgentRuntimeContextSection } from './AgentRuntimeContextSection'
import { AgentBuiltInCapabilitySection, AgentContextSection, AgentImageModelSection, AgentModelOnlySection, AgentModelSection, AgentPromptSection, AgentSkillSection, AgentToolSection, mergeAgentModelOverride, mergeAgentPromptOverride } from './agent-configuration-sections'
import { AGENTS, toolDefinitionsFromManifest } from './agent-registry'
import type { ToolKey, VisibleAgentKey } from './agent-registry'
import {
  AgentHeader,
  AgentList,
  CreateCustomAgentDialog,
  CustomAgentIdentitySection,
  findCustomAgent,
  isVisibleCustomAgent,
  mergeCustomAgentViews,
  updateCustomAgent,
  type AgentSelectionID,
} from './custom-agent-management'
import type { ToolNavigationIntent } from '@/components/Chat/tool-navigation'

const tabCls = 'nova-nav-item rounded-[var(--nova-radius)] px-2.5 py-1 text-xs'
export function AgentsView({ target, onClose, toolNavigationIntent }: { target: ResourceTarget; onClose?: () => void; toolNavigationIntent?: ToolNavigationIntent | null }) {
  const { t } = useTranslation()
  const targetKind = target.kind
  const projectId = target.kind === 'project' ? target.projectId : ''
  const resourceTarget = useMemo<ResourceTarget>(
    () => targetKind === 'project' ? { kind: 'project', projectId } : { kind: 'global' },
    [projectId, targetKind],
  )
  const targetKey = resourceTargetKey(resourceTarget)
  const agentAvailable = targetKind === 'project'
  const [selectedLayer, setActiveLayer] = useState<SettingsLayer>('user')
  const activeLayer: SettingsLayer = targetKind === 'project' ? selectedLayer : 'user'
  const { layered, draft, setDraft, error, autosaveStatus, autosaveError, reload, saveNow } = useLayeredSettingsDraft({
    target: resourceTarget,
    layer: activeLayer,
    sourcePrefix: 'agents-view',
  })
  const [activeSelection, setActiveSelection] = useState<AgentSelectionID>('ide')
  const [createOpen, setCreateOpen] = useState(false)
  const [createBaseKind, setCreateBaseKind] = useState<CustomAgentBaseKind>('ide')
  const [skills, setSkills] = useState<SkillSummary[]>([])
  const [agentChatOpen, setAgentChatOpen] = useState(false)
  const [sidebarVisible, setSidebarVisible] = useState(true)
  const toolNavigationNonceRef = useRef(0)

  useEffect(() => {
    const intent = toolNavigationIntent
    if (!intent || intent.nonce === toolNavigationNonceRef.current || intent.target.kind !== 'config_resource' || intent.target.resource !== 'agent_profile') return
    toolNavigationNonceRef.current = intent.nonce
    const fixedAgent = AGENTS.find((agent) => agent.key === intent.target.id)
    if (fixedAgent) setActiveSelection(fixedAgent.key)
    else if (layered?.effective.custom_agents?.some((agent) => agent.id === intent.target.id)) setActiveSelection(`custom:${intent.target.id}`)
    if (intent.target.scope === 'workspace' && targetKind === 'project') setActiveLayer('workspace')
    else if (intent.target.scope === 'user') setActiveLayer('user')
  }, [layered?.effective.custom_agents, targetKind, toolNavigationIntent?.nonce])

  useEffect(() => {
    let cancelled = false
    const loadSkills = () => {
      getSkills(resourceTarget)
        .then((snapshot) => {
          if (!cancelled) setSkills(snapshot.skills.filter((skill) => skill.active))
        })
        .catch((error) => {
          if (!cancelled) console.warn('[agents] load skills failed', error)
        })
    }
    const onSkillsUpdated = (event: Event) => {
      const changedTargetKey = (event as CustomEvent<{ targetKey?: string }>).detail?.targetKey
      if (changedTargetKey && changedTargetKey !== targetKey && changedTargetKey !== 'global') return
      loadSkills()
    }
    loadSkills()
    window.addEventListener('nova:skills-updated', onSkillsUpdated)
    return () => {
      cancelled = true
      window.removeEventListener('nova:skills-updated', onSkillsUpdated)
    }
  }, [resourceTarget, targetKey])

  const effective = layered?.effective ?? {}
  const customAgentID = activeSelection.startsWith('custom:') ? activeSelection.slice('custom:'.length) : ''
  const effectiveCustomAgents = mergeCustomAgentViews(effective.custom_agents, draft.custom_agents).filter(isVisibleCustomAgent)
  const selectedCustomAgent = effectiveCustomAgents.find((agent) => agent.id === customAgentID)
  const activeAgent = (selectedCustomAgent?.base_kind ?? (customAgentID ? 'ide' : activeSelection)) as VisibleAgentKey
  const selected = AGENTS.find((agent) => agent.key === activeAgent) ?? AGENTS[0]
  const layerCustomAgent = findCustomAgent(draft, customAgentID)
  const inheritedSettings = layered?.inherited?.[activeLayer] ?? {}
  const inheritedCustomAgent = findCustomAgent(inheritedSettings, customAgentID)
  const profileOptions = useMemo(() => buildProfileOptions(draft, effective, t), [draft, effective, t])
  const imageProfileOptions = useMemo(() => buildImageProfileOptions(draft, effective, t), [draft, effective, t])
  const baseInheritedModel = mergeAgentModelOverride(inheritedSettings.agent_models?.default ?? {}, inheritedSettings.agent_models?.[activeAgent] ?? {})
  const modelValue = selectedCustomAgent ? layerCustomAgent?.model ?? {} : draft.agent_models?.[activeAgent] ?? {}
  const inheritedModel = selectedCustomAgent ? mergeAgentModelOverride(baseInheritedModel, inheritedCustomAgent?.model ?? {}) : baseInheritedModel
  const baseInheritedPrompt = mergeAgentPromptOverride(inheritedSettings.agent_prompts?.default ?? {}, inheritedSettings.agent_prompts?.[activeAgent] ?? {})
  const promptValue = selectedCustomAgent ? layerCustomAgent?.prompt ?? {} : draft.agent_prompts?.[activeAgent] ?? {}
  const inheritedPrompt = selectedCustomAgent ? mergeAgentPromptOverride(baseInheritedPrompt, inheritedCustomAgent?.prompt ?? {}) : baseInheritedPrompt
  const toolValue = selectedCustomAgent ? layerCustomAgent?.tools ?? {} : draft.agent_tools?.[activeAgent] ?? {}
  const resolvedToolManifest = layered?.resolved_agent_tool_manifests?.[selectedCustomAgent?.id ?? activeAgent]
  const toolRows = useMemo(() => toolDefinitionsFromManifest(resolvedToolManifest), [resolvedToolManifest])
  const skillsAllowed = Boolean(toolValue.skills ?? toolRows.find((tool) => tool.key === 'skills')?.allowed ?? false)
  const skillValue = selectedCustomAgent ? layerCustomAgent?.skills ?? {} : draft.agent_skills?.[activeAgent] ?? {}
  const effectiveSkillSettings = selectedCustomAgent ? {
    ...(effective.agent_skills ?? {}),
    [activeAgent]: { ...(effective.agent_skills?.[activeAgent] ?? {}), ...(selectedCustomAgent.skills ?? {}) },
  } : effective.agent_skills
  const contextValue = selectedCustomAgent ? layerCustomAgent?.context ?? {} : draft.agent_context?.[activeAgent] ?? {}
  const resolvedContext = layered?.resolved_agent_contexts?.[selectedCustomAgent?.id ?? activeAgent]
  const inheritedImageProfileID = selectedCustomAgent
    ? inheritedCustomAgent?.image_api_profile_id || resolveInheritedImageProfileID(layered, activeLayer)
    : resolveInheritedImageProfileID(layered, activeLayer)
  const inheritedToolParallelism = resolveInheritedToolParallelism(layered, activeLayer)
  const inheritedSubAgentParallelism = resolveInheritedSubAgentParallelism(layered, activeLayer)
  const activeAgentTitle = selectedCustomAgent?.name || t(selected.titleKey)
  const configurationContext = useMemo(() => ({
    active_settings_layer: activeLayer,
    active_agent: selectedCustomAgent?.id ?? activeAgent,
    active_agent_title: activeAgentTitle,
    write_scope_required: 'true',
    write_scope_hint: activeLayer,
  }), [activeAgent, activeAgentTitle, activeLayer, selectedCustomAgent?.id])

  const reloadAfterAgentMutation = useCallback(() => {
    void saveNow()
      .then(async () => {
        await reload()
      })
      .catch(() => undefined)
  }, [reload, saveNow])

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
    if (selectedCustomAgent) {
      setDraft((current) => updateCustomAgent(current, selectedCustomAgent, (agent) => ({ ...agent, model: { ...(agent.model ?? {}), ...patch } })))
      return
    }
    setDraft((current) => ({
      ...current,
      agent_models: {
        ...(current.agent_models ?? {}),
        [activeAgent]: { ...(current.agent_models?.[activeAgent] ?? {}), ...patch },
      },
    }))
  }

  const setAgentTool = (key: ToolKey, value: boolean | null) => {
    if (selectedCustomAgent) {
      setDraft((current) => updateCustomAgent(current, selectedCustomAgent, (agent) => {
        const tools: AgentToolOverride = { ...(agent.tools ?? {}) }
        if (value === null) delete tools[key]
        else tools[key] = value
        return { ...agent, tools }
      }))
      return
    }
    setDraft((current) => {
      const nextAgentTools = { ...(current.agent_tools ?? {}) }
      const nextOverrides: AgentToolOverride = { ...(nextAgentTools[activeAgent] ?? {}) }
      if (value === null) delete nextOverrides[key]
      else nextOverrides[key] = value
      nextAgentTools[activeAgent] = nextOverrides
      return { ...current, agent_tools: nextAgentTools }
    })
  }

  const setAgentSkill = (name: string, value: boolean | null) => {
    if (selectedCustomAgent) {
      setDraft((current) => updateCustomAgent(current, selectedCustomAgent, (agent) => {
        const agentSkills: AgentSkillOverride = { ...(agent.skills ?? {}) }
        if (value === null) delete agentSkills[name]
        else agentSkills[name] = value
        return { ...agent, skills: agentSkills }
      }))
      return
    }
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
    if (selectedCustomAgent) {
      setDraft((current) => updateCustomAgent(current, selectedCustomAgent, (agent) => ({ ...agent, context: { ...(agent.context ?? {}), ...patch } })))
      return
    }
    setDraft((current) => ({
      ...current,
      agent_context: {
        ...(current.agent_context ?? {}),
        [activeAgent]: { ...(current.agent_context?.[activeAgent] ?? {}), ...patch },
      },
    }))
  }

  const setToolParallelism = (value: number | null) => {
    setDraft((current) => ({ ...current, agent_tool_parallelism: value }))
  }

  const setSubAgentParallelism = (value: number | null) => {
    setDraft((current) => ({ ...current, agent_subagent_parallelism: value }))
  }

  const setImageProfile = (profileID: string) => {
    if (selectedCustomAgent) {
      setDraft((current) => updateCustomAgent(current, selectedCustomAgent, (agent) => ({ ...agent, image_api_profile_id: profileID })))
      return
    }
    setDraft((current) => ({ ...current, default_image_api_profile_id: profileID }))
  }

  const setAgentPrompt = (patch: Partial<AgentPromptOverride>) => {
    if (selectedCustomAgent) {
      setDraft((current) => updateCustomAgent(current, selectedCustomAgent, (agent) => ({ ...agent, prompt: { ...(agent.prompt ?? {}), ...patch } })))
      return
    }
    setDraft((current) => ({
      ...current,
      agent_prompts: {
        ...(current.agent_prompts ?? {}),
        [activeAgent]: { ...(current.agent_prompts?.[activeAgent] ?? {}), ...patch },
      },
    }))
  }

  const archiveCustomAgent = () => {
    if (!selectedCustomAgent) return
    setDraft((current) => updateCustomAgent(current, selectedCustomAgent, (agent) => ({ ...agent, enabled: false })))
    setActiveSelection(selectedCustomAgent.base_kind ?? 'ide')
  }

  const setCustomIdentity = (patch: Partial<Pick<CustomAgentConfig, 'name' | 'description'>>) => {
    if (!selectedCustomAgent) return
    setDraft((current) => updateCustomAgent(current, selectedCustomAgent, (agent) => ({ ...agent, ...patch })))
  }

  return (
    <FeaturePageShell
      icon={Bot}
      title="Agents"
      leadingContent={(
        <SidebarVisibilityToggle
          visible={sidebarVisible}
          onToggle={() => setSidebarVisible((visible) => !visible)}
        />
      )}
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
          {(targetKind === 'project' ? ['user', 'workspace'] as SettingsLayer[] : ['user'] as SettingsLayer[]).map((layer) => (
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
          {agentAvailable && (
            <ConfigManagerToggle
              open={agentChatOpen}
              label={t('agents.configAgent.button')}
              onToggle={() => setAgentChatOpen((value) => !value)}
            />
          )}
        </>
      )}
    >
      {!layered ? (
        <LoadingState label={t('common.loading')} className="min-h-0 flex-1" />
      ) : (
      <AdaptiveSurface
        left={{
          id: 'agents-list',
          title: 'Agents',
          side: 'left',
          icon: <Bot className="h-4 w-4" />,
          content: (
            <AgentList
              active={activeSelection}
              customAgents={effectiveCustomAgents}
              onSelect={setActiveSelection}
              onCreate={(baseKind) => {
                setCreateBaseKind(baseKind)
                setCreateOpen(true)
              }}
            />
          ),
          desktopClassName: 'min-h-0 border-r border-[var(--nova-border)]',
          desktopVisible: sidebarVisible,
          mobileClassName: 'w-[min(88vw,340px)]',
        }}
        right={agentAvailable && agentChatOpen ? {
          id: 'agents-config-manager',
          title: t('agents.configAgent.title'),
          side: 'right',
          icon: <Bot className="h-4 w-4" />,
          content: (
            <div className="h-full min-h-0 bg-[var(--nova-surface)]">
              <ConfigManagerChat
                projectId={projectId}
                origin="agents"
                resourceId={`${activeLayer}:${activeAgent}`}
                context={configurationContext}
                onMutated={reloadAfterAgentMutation}
              />
            </div>
          ),
          desktopClassName: 'min-h-0 border-l border-[var(--nova-border)]',
          mobileClassName: 'w-[min(92vw,420px)]',
        } : undefined}
        className="flex-1 text-xs"
        mainClassName="min-h-0 min-w-0"
        leftResize={{
          layoutKey: 'nova-agents-list-layout',
          label: t('layout.resize.sidebar'),
          defaultSize: '288px',
          minSize: '220px',
          maxSize: '40%',
        }}
        rightResize={{
          layoutKey: 'nova-agents-config-manager-layout',
          label: t('layout.resize.right'),
          defaultSize: '420px',
          minSize: '300px',
          maxSize: '65%',
          mainMinSize: '240px',
        }}
      >
        {({ isMobile, openLeft, openRight }) => (
          <main className="h-full min-h-0 overflow-y-auto overflow-x-hidden">
            {isMobile && (
              <div className="sticky top-0 z-10 flex h-10 items-center gap-2 border-b border-[var(--nova-border)] bg-[var(--nova-surface)] px-3">
                <MobilePaneTrigger side="left" label={t('workbench.mobile.openSidePanel', { label: 'Agents' })} onClick={openLeft} />
                <span className="min-w-0 truncate text-[11px] text-[var(--nova-text-muted)]">{activeAgentTitle}</span>
                {agentAvailable && agentChatOpen && (
                  <MobilePaneTrigger side="right" label={t('workbench.mobile.openSidePanel', { label: t('agents.configAgent.title') })} onClick={openRight} className="ml-auto" />
                )}
              </div>
            )}
            <div className="mx-auto flex w-full min-w-0 max-w-5xl flex-col gap-5 px-4 py-5 sm:px-6">
              <AgentHeader agent={selected} customAgent={selectedCustomAgent} onArchive={selectedCustomAgent ? archiveCustomAgent : undefined} />
              {selectedCustomAgent ? (
                <CustomAgentIdentitySection
                  agent={selectedCustomAgent}
                  value={layerCustomAgent}
                  onChange={setCustomIdentity}
                />
              ) : null}
              <AgentToolSchedulingSection
                toolValue={draft.agent_tool_parallelism ?? null}
                inheritedToolValue={inheritedToolParallelism}
                onToolChange={setToolParallelism}
                subAgentValue={draft.agent_subagent_parallelism ?? null}
                inheritedSubAgentValue={inheritedSubAgentParallelism}
                onSubAgentChange={setSubAgentParallelism}
              />
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
              {activeAgent === 'image' && activeLayer === 'user' && (
                <AgentImageModelSection
                  value={selectedCustomAgent ? layerCustomAgent?.image_api_profile_id ?? '' : draft.default_image_api_profile_id ?? ''}
                  inherited={inheritedImageProfileID}
                  profiles={imageProfileOptions}
                  onChange={setImageProfile}
                />
              )}
              {resolvedContext && (
                <AgentRuntimeContextSection
                  value={contextValue}
                  resolved={resolvedContext}
                  onChange={setAgentContext}
                />
              )}
              <AgentPromptSection
                value={promptValue}
                inherited={inheritedPrompt}
                builtin={layered.builtin_agent_prompts?.[activeAgent]?.system_prompt ?? ''}
                blocks={layered.builtin_agent_prompt_blocks?.[activeAgent]}
                sources={layered.builtin_agent_prompt_sources?.[activeAgent]?.sources}
                onChange={setAgentPrompt}
              />
              {selected.capabilityMode === 'tools' ? (
                <>
                  <AgentToolSection
                    value={toolValue}
                    rows={toolRows}
                    onChange={setAgentTool}
                  />
                  {skillsAllowed && (
                    <AgentSkillSection
                      agent={activeAgent}
                      skills={skills}
                      value={skillValue}
                      effective={effectiveSkillSettings}
                      onChange={setAgentSkill}
                    />
                  )}
                </>
              ) : selected.capabilityMode === 'built_in' ? (
                <AgentBuiltInCapabilitySection agent={selected.key} />
              ) : (
                <AgentModelOnlySection />
              )}
              <AgentContextSection agent={selected.key} effective={effective} resolved={resolvedContext} />
            </div>
          </main>
        )}
      </AdaptiveSurface>
      )}
      <CreateCustomAgentDialog
        open={createOpen}
        initialBaseKind={createBaseKind}
        onOpenChange={setCreateOpen}
        onCreate={(agent) => {
          setDraft((current) => ({ ...current, custom_agents: [...(current.custom_agents ?? []), agent] }))
          setActiveSelection(`custom:${agent.id}`)
        }}
      />
    </FeaturePageShell>
  )
}

function AgentToolSchedulingSection({
  toolValue,
  inheritedToolValue,
  onToolChange,
  subAgentValue,
  inheritedSubAgentValue,
  onSubAgentChange,
}: {
  toolValue: number | null
  inheritedToolValue: number
  onToolChange: (value: number | null) => void
  subAgentValue: number | null
  inheritedSubAgentValue: number
  onSubAgentChange: (value: number | null) => void
}) {
  const { t } = useTranslation()
  const toolInputID = useId()
  const subAgentInputID = useId()
  return (
    <section className="border-b border-[var(--nova-border)] pb-5">
      <h2 className="mb-3 text-xs font-semibold uppercase tracking-[0.12em] text-[var(--nova-text-muted)]">
        {t('agents.section.toolScheduling')}
      </h2>
      <SettingsFieldRow
        htmlFor={toolInputID}
        title={t('settings.agent.toolParallelism')}
        description={t('agents.tool.parallelismNote')}
        meta={toolValue === null ? <span className="text-[10px] text-[var(--nova-text-faint)]">{t('common.inherit', { value: inheritedToolValue })}</span> : undefined}
        controlClassName="sm:w-36"
      >
        <Input
          id={toolInputID}
          type="number"
          min={1}
          max={64}
          value={toolValue ?? ''}
          placeholder={String(inheritedToolValue)}
          aria-label={t('settings.agent.toolParallelism')}
          onChange={(event) => {
            if (event.target.value === '') {
              onToolChange(null)
              return
            }
            const parsed = Number(event.target.value)
            if (Number.isFinite(parsed)) onToolChange(Math.min(64, Math.max(1, Math.trunc(parsed))))
          }}
        />
      </SettingsFieldRow>
      <SettingsFieldRow
        htmlFor={subAgentInputID}
        title={t('settings.agent.subAgentParallelism')}
        description={t('agents.tool.subAgentParallelismNote')}
        meta={subAgentValue === null ? <span className="text-[10px] text-[var(--nova-text-faint)]">{t('common.inherit', { value: inheritedSubAgentValue })}</span> : undefined}
        controlClassName="sm:w-36"
      >
        <Input
          id={subAgentInputID}
          type="number"
          min={1}
          max={32}
          value={subAgentValue ?? ''}
          placeholder={String(inheritedSubAgentValue)}
          aria-label={t('settings.agent.subAgentParallelism')}
          onChange={(event) => {
            if (event.target.value === '') {
              onSubAgentChange(null)
              return
            }
            const parsed = Number(event.target.value)
            if (Number.isFinite(parsed)) onSubAgentChange(Math.min(32, Math.max(1, Math.trunc(parsed))))
          }}
        />
      </SettingsFieldRow>
    </section>
  )
}

function resolveInheritedToolParallelism(layered: LayeredSettings | null, layer: SettingsLayer) {
  const layers = layer === 'workspace'
    ? [layered?.default, layered?.global, layered?.user]
    : [layered?.default, layered?.global]
  let value = 8
  for (const settings of layers) {
    const candidate = settings?.agent_tool_parallelism
    if (candidate === null || candidate === undefined) continue
    value = candidate <= 0 ? 8 : Math.min(64, Math.trunc(candidate))
  }
  return value
}

function resolveInheritedSubAgentParallelism(layered: LayeredSettings | null, layer: SettingsLayer) {
  const layers = layer === 'workspace'
    ? [layered?.default, layered?.global, layered?.user]
    : [layered?.default, layered?.global]
  let value = 4
  for (const settings of layers) {
    const candidate = settings?.agent_subagent_parallelism
    if (candidate === null || candidate === undefined) continue
    value = candidate <= 0 ? 4 : Math.min(32, Math.trunc(candidate))
  }
  return value
}

function resolveInheritedImageProfileID(layered: LayeredSettings | null, layer: SettingsLayer) {
  const layers = layer === 'workspace'
    ? [layered?.default, layered?.global, layered?.user]
    : [layered?.default, layered?.global]
  let value = 'default'
  for (const settings of layers) {
    const candidate = settings?.default_image_api_profile_id?.trim()
    if (candidate) value = candidate
  }
  return value
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

function buildImageProfileOptions(draft: Settings, effective: Settings, t: (key: string, options?: Record<string, unknown>) => string): Array<{ id: string; label: string }> {
  const profiles = new Map<string, string>()
  const add = (profile?: ImageAPIProfileSettings) => {
    const id = imageAPIProfileID(profile)
    if (!id) return
    profiles.set(id, imageAPIProfileLabel(profile))
  }
  imageAPIProfilesWithDefault(effective).forEach(add)
  ;(draft.image_api_profiles ?? []).forEach(add)
  return Array.from(profiles.entries()).map(([id, label]) => ({
    id,
    label: id === 'default' ? t('agents.option.defaultProfile', { label }) : t('agents.option.profile', { id, label }),
  }))
}
