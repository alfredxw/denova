import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Bot } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { ConfigManagerChat } from '@/components/Chat/ConfigManagerChat'
import { ConfigManagerToggle } from '@/components/Chat/ConfigManagerToggle'
import { AutosaveStatusIndicator } from '@/components/forms/autosave-status'
import { AdaptiveSurface } from '@/components/layout/adaptive-surface'
import { FeaturePageShell } from '@/components/layout/feature-page-shell'
import { MobilePaneTrigger } from '@/components/layout/mobile-pane-trigger'
import { SidebarVisibilityToggle } from '@/components/layout/sidebar-visibility-toggle'
import { Button } from '@/components/ui/button'
import { LoadingState } from '@/components/common/LoadingState'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import type { AgentContextOverride, AgentDelegationPolicy, AgentModelOverride, AgentPromptOverride, AgentRuntimeKind, AgentSkillOverride, AgentSkillPolicy, AgentToolOverride, CustomAgentConfig, Settings, SettingsLayer } from '@/features/settings/types'
import { useLayeredSettingsDraft } from '@/features/settings/use-layered-settings-draft'
import { getSkills, resourceTargetKey } from '@/lib/api'
import type { ResourceTarget, SkillSummary } from '@/lib/api'
import { AgentRuntimeContextSection } from './AgentRuntimeContextSection'
import { AgentBuiltInCapabilitySection, AgentContextSection, AgentImageModelSection, AgentModelOnlySection, AgentModelSection, AgentPromptSection, AgentToolSection, mergeAgentModelOverride, mergeAgentPromptOverride } from './agent-configuration-sections'
import { AGENTS, toolDefinitionsFromManifest } from './agent-registry'
import type { SubAgentParentKey, ToolKey, VisibleAgentKey } from './agent-registry'
import { AgentSubAgentSection } from './agent-subagent-section'
import { contractForRuntimeKind, runtimeKindForContract } from './agent-contracts'
import { buildImageProfileOptions, buildProfileOptions, cloneBuiltInAgent, resolveInheritedImageProfileID, resolveInheritedSubAgentParallelism, resolveInheritedToolParallelism, skillOverrideToPolicy } from './agent-definition-state'
import { AgentToolSchedulingSection } from './agent-tool-scheduling-section'
import { AgentContextBindingsSection, AgentDelegationPolicySection, AgentSkillPolicySection, AgentToolGuidanceSection, CustomAgentBehaviorSection, ResolvedAgentSection } from './custom-agent-definition-sections'
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
type AgentSection = 'overview' | 'behavior' | 'capabilities' | 'context' | 'resolved'
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
  const [activeSelection, setActiveSelection] = useState<AgentSelectionID>('ide')
  const [selectedLayer, setActiveLayer] = useState<SettingsLayer>('user')
  const customSelection = activeSelectionIDIsCustom(activeSelection)
  const activeLayer: SettingsLayer = customSelection ? 'user' : (targetKind === 'project' ? selectedLayer : 'user')
  const { layered, draft, setDraft, error, autosaveStatus, autosaveError, reload, saveNow } = useLayeredSettingsDraft({
    target: resourceTarget,
    layer: activeLayer,
    sourcePrefix: 'agents-view',
  })
  const [createOpen, setCreateOpen] = useState(false)
  const [createRuntimeKind, setCreateRuntimeKind] = useState<AgentRuntimeKind>('ide')
  const [skills, setSkills] = useState<SkillSummary[]>([])
  const [agentChatOpen, setAgentChatOpen] = useState(false)
  const [sidebarVisible, setSidebarVisible] = useState(true)
  const [activeSection, setActiveSection] = useState<AgentSection>('overview')
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

  useEffect(() => setActiveSection('overview'), [activeSelection])

  const effective = layered?.effective ?? {}
  const customAgentID = activeSelection.startsWith('custom:') ? activeSelection.slice('custom:'.length) : ''
  const effectiveCustomAgents = mergeCustomAgentViews(effective.custom_agents, draft.custom_agents).filter(isVisibleCustomAgent)
  const selectedCustomAgent = effectiveCustomAgents.find((agent) => agent.id === customAgentID)
  const activeAgent = (runtimeKindForContract(selectedCustomAgent?.contract) ?? (customAgentID ? 'ide' : activeSelection)) as VisibleAgentKey
  const selected = AGENTS.find((agent) => agent.key === activeAgent) ?? AGENTS[0]
  const layerCustomAgent = findCustomAgent(draft, customAgentID)
  const inheritedSettings = layered?.inherited?.[activeLayer] ?? {}
  const inheritedCustomAgent = findCustomAgent(inheritedSettings, customAgentID)
  const profileOptions = useMemo(() => buildProfileOptions(draft, effective, t), [draft, effective, t])
  const imageProfileOptions = useMemo(() => buildImageProfileOptions(draft, effective, t), [draft, effective, t])
  const baseInheritedModel = mergeAgentModelOverride(inheritedSettings.agent_models?.default ?? {}, inheritedSettings.agent_models?.[activeAgent] ?? {})
  const modelValue = selectedCustomAgent ? layerCustomAgent?.model ?? selectedCustomAgent.model ?? {} : draft.agent_models?.[activeAgent] ?? {}
  const inheritedModel = selectedCustomAgent ? {} : baseInheritedModel
  const baseInheritedPrompt = mergeAgentPromptOverride(inheritedSettings.agent_prompts?.default ?? {}, inheritedSettings.agent_prompts?.[activeAgent] ?? {})
  const promptValue = draft.agent_prompts?.[activeAgent] ?? {}
  const inheritedPrompt = baseInheritedPrompt
  const toolValue = selectedCustomAgent ? layerCustomAgent?.tools ?? selectedCustomAgent.tools ?? {} : draft.agent_tools?.[activeAgent] ?? {}
  const resolvedToolManifest = layered?.resolved_agent_tool_manifests?.[selectedCustomAgent?.id ?? activeAgent]
    ?? layered?.resolved_agent_tool_manifests?.[activeAgent]
  const toolRows = useMemo(() => toolDefinitionsFromManifest(resolvedToolManifest), [resolvedToolManifest])
  const configuredToolRows = useMemo(() => toolRows.map((row) => ({ ...row, allowed: toolValue[row.key] ?? row.allowed })), [toolRows, toolValue])
  const skillsAllowed = Boolean(configuredToolRows.find((tool) => tool.key === 'skills')?.allowed ?? false)
  const skillValue = draft.agent_skills?.[activeAgent] ?? {}
  const builtInSkillPolicy = skillOverrideToPolicy(skillValue)
  const contextValue = selectedCustomAgent ? layerCustomAgent?.runtime_context ?? selectedCustomAgent.runtime_context ?? {} : draft.agent_context?.[activeAgent] ?? {}
  const resolvedContext = layered?.resolved_agent_contexts?.[selectedCustomAgent?.id ?? activeAgent]
    ?? layered?.resolved_agent_contexts?.[activeAgent]
  const resolvedDefinition = layered?.resolved_agent_definitions?.[selectedCustomAgent?.id ?? activeAgent]
  const promptSources = layered?.builtin_agent_prompt_sources?.[activeAgent]?.sources
  const runtimeContract = promptSources?.find((source) => source.id === 'runtime_contract')?.content
  const outputProtocol = promptSources?.find((source) => source.id === 'output_protocol')?.content
  const customSkillPolicy = layerCustomAgent?.skill_policy ?? selectedCustomAgent?.skill_policy ?? { mode: 'managed' }
  const customDelegation = layerCustomAgent?.delegation ?? selectedCustomAgent?.delegation ?? { mode: 'compatible' }
  const customContextBindings = layerCustomAgent?.context_bindings ?? selectedCustomAgent?.context_bindings ?? []
  const customToolGuidance = layerCustomAgent?.tool_guidance ?? selectedCustomAgent?.tool_guidance ?? {}
  const subAgentParent = isSubAgentParent(activeAgent) ? activeAgent as SubAgentParentKey : undefined
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

  const setAgentContext = (patch: Partial<AgentContextOverride>) => {
    if (selectedCustomAgent) {
      setDraft((current) => updateCustomAgent(current, selectedCustomAgent, (agent) => ({ ...agent, runtime_context: { ...(agent.runtime_context ?? {}), ...patch } })))
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

  const selectAgent = (selection: AgentSelectionID) => {
    if (activeLayer === 'workspace' && activeSelectionIDIsCustom(selection)) {
      void saveNow().then(() => {
        setActiveLayer('user')
        setActiveSelection(selection)
      }).catch(() => undefined)
      return
    }
    setActiveSelection(selection)
  }

  const openCreateAgent = (runtimeKind: AgentRuntimeKind) => {
    const open = () => {
      setActiveLayer('user')
      setCreateRuntimeKind(runtimeKind)
      setCreateOpen(true)
    }
    if (activeLayer === 'workspace') {
      void saveNow().then(open).catch(() => undefined)
      return
    }
    open()
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
    setActiveSelection(runtimeKindForContract(selectedCustomAgent.contract) ?? 'ide')
  }

  const setCustomIdentity = (patch: Partial<Pick<CustomAgentConfig, 'name' | 'description'>>) => {
    if (!selectedCustomAgent) return
    setDraft((current) => updateCustomAgent(current, selectedCustomAgent, (agent) => ({ ...agent, ...patch })))
  }

  const updateSelectedCustomAgent = (mutate: (agent: CustomAgentConfig) => CustomAgentConfig) => {
    if (!selectedCustomAgent) return
    setDraft((current) => updateCustomAgent(current, selectedCustomAgent, mutate))
  }

  const setCustomSkillPolicy = (policy: AgentSkillPolicy) => updateSelectedCustomAgent((agent) => ({ ...agent, skill_policy: policy }))
  const setCustomDelegation = (delegation: AgentDelegationPolicy) => updateSelectedCustomAgent((agent) => ({ ...agent, delegation }))

  const setBuiltInSkillPolicy = (policy: AgentSkillPolicy) => {
    const next = new Map<string, boolean>()
    for (const name of policy.pinned ?? []) next.set(name, true)
    for (const name of policy.blocked ?? []) next.set(name, false)
    setDraft((current) => ({
      ...current,
      agent_skills: { ...(current.agent_skills ?? {}), [activeAgent]: Object.fromEntries(next) as AgentSkillOverride },
    }))
  }

  const setGeneralSubAgent = (agent: SubAgentParentKey, value: boolean | null) => {
    setDraft((current) => ({
      ...current,
      general_sub_agents: { ...(current.general_sub_agents ?? {}), [agent]: value },
    }))
  }

  const setSubAgents = (updater: (current: NonNullable<Settings['sub_agents']>) => NonNullable<Settings['sub_agents']>) => {
    setDraft((current) => ({ ...current, sub_agents: updater(current.sub_agents ?? []) }))
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
          {(targetKind === 'project' && !selectedCustomAgent ? ['user', 'workspace'] as SettingsLayer[] : ['user'] as SettingsLayer[]).map((layer) => (
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
              onSelect={selectAgent}
              onCreate={openCreateAgent}
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
              <Tabs value={activeSection} onValueChange={(value) => setActiveSection(value as AgentSection)} className="min-w-0 gap-5">
                <div className="overflow-x-auto border-b border-[var(--nova-border)]">
                  <TabsList variant="line" className="h-9 min-w-max">
                    {(['overview', 'behavior', 'capabilities', 'context', 'resolved'] as AgentSection[]).map((section) => (
                      <TabsTrigger key={section} value={section} className="px-3 text-xs">{t(`agents.tab.${section}`)}</TabsTrigger>
                    ))}
                  </TabsList>
                </div>
                <TabsContent value="overview" className="flex flex-col gap-5">
                  {selectedCustomAgent ? <CustomAgentIdentitySection agent={selectedCustomAgent} value={layerCustomAgent} onChange={setCustomIdentity} /> : null}
                  <AgentToolSchedulingSection
                    toolValue={draft.agent_tool_parallelism ?? null}
                    inheritedToolValue={inheritedToolParallelism}
                    onToolChange={setToolParallelism}
                    subAgentValue={draft.agent_subagent_parallelism ?? null}
                    inheritedSubAgentValue={inheritedSubAgentParallelism}
                    onSubAgentChange={setSubAgentParallelism}
                  />
                  {activeLayer === 'user' ? <AgentModelSection value={modelValue} inherited={inheritedModel} profiles={profileOptions} onChange={setAgentModel} /> : (
                    <section className="border-b border-[var(--nova-border)] pb-5 text-xs text-[var(--nova-text-muted)]">{t('agents.model.userScoped')}</section>
                  )}
                  {activeAgent === 'image' && activeLayer === 'user' ? <AgentImageModelSection
                    value={selectedCustomAgent ? layerCustomAgent?.image_api_profile_id ?? selectedCustomAgent.image_api_profile_id ?? '' : draft.default_image_api_profile_id ?? ''}
                    inherited={inheritedImageProfileID}
                    profiles={imageProfileOptions}
                    onChange={setImageProfile}
                  /> : null}
                </TabsContent>
                <TabsContent value="behavior" className="flex flex-col gap-5">
                  {selectedCustomAgent ? <CustomAgentBehaviorSection
                    instructions={layerCustomAgent?.instructions ?? selectedCustomAgent.instructions ?? ''}
                    runtimeContract={runtimeContract}
                    outputProtocol={outputProtocol}
                    onChange={(instructions) => updateSelectedCustomAgent((agent) => ({ ...agent, instructions }))}
                  /> : <AgentPromptSection
                    value={promptValue}
                    inherited={inheritedPrompt}
                    builtin={layered.builtin_agent_prompts?.[activeAgent]?.system_prompt ?? ''}
                    blocks={layered.builtin_agent_prompt_blocks?.[activeAgent]}
                    sources={promptSources}
                    onChange={setAgentPrompt}
                  />}
                </TabsContent>
                <TabsContent value="capabilities" className="flex flex-col gap-5">
                  {selected.capabilityMode === 'tools' ? <>
                    <AgentToolSection value={toolValue} rows={configuredToolRows} onChange={setAgentTool} />
                    {selectedCustomAgent ? <AgentToolGuidanceSection rows={configuredToolRows} value={customToolGuidance} onChange={(tool_guidance) => updateSelectedCustomAgent((agent) => ({ ...agent, tool_guidance }))} /> : null}
                    {skillsAllowed ? <AgentSkillPolicySection
                      skills={skills}
                      value={selectedCustomAgent ? customSkillPolicy : builtInSkillPolicy}
                      allowExplicit={Boolean(selectedCustomAgent)}
                      onChange={selectedCustomAgent ? setCustomSkillPolicy : setBuiltInSkillPolicy}
                    /> : null}
                    {selectedCustomAgent ? <AgentDelegationPolicySection value={customDelegation} runtimeKind={activeAgent} subAgents={effective.sub_agents ?? []} onChange={setCustomDelegation} /> : null}
                    {!selectedCustomAgent && subAgentParent ? <AgentSubAgentSection
                      agent={subAgentParent}
                      toolRows={configuredToolRows}
                      generalSettings={draft.general_sub_agents}
                      effectiveGeneralSettings={effective.general_sub_agents}
                      subAgents={draft.sub_agents ?? []}
                      effectiveSubAgents={effective.sub_agents ?? []}
                      profiles={profileOptions}
                      onGeneralChange={setGeneralSubAgent}
                      onChange={setSubAgents}
                    /> : null}
                  </> : selected.capabilityMode === 'built_in' ? <AgentBuiltInCapabilitySection agent={selected.key} /> : <AgentModelOnlySection />}
                </TabsContent>
                <TabsContent value="context" className="flex flex-col gap-5">
                  {resolvedContext ? <AgentRuntimeContextSection value={contextValue} resolved={resolvedContext} onChange={setAgentContext} /> : null}
                  {selectedCustomAgent ? <AgentContextBindingsSection value={customContextBindings} onChange={(context_bindings) => updateSelectedCustomAgent((agent) => ({ ...agent, context_bindings }))} /> : null}
                  <AgentContextSection agent={selected.key} effective={effective} resolved={resolvedContext} />
                </TabsContent>
                <TabsContent value="resolved" className="flex flex-col gap-5">
                  {(selectedCustomAgent || isCustomizableRuntimeKind(activeAgent)) ? <ResolvedAgentSection
                    agent={selectedCustomAgent ?? { id: activeAgent, contract: contractForRuntimeKind(activeAgent as AgentRuntimeKind) }}
                    resolved={resolvedDefinition}
                    tools={configuredToolRows}
                  /> : <AgentModelOnlySection />}
                </TabsContent>
              </Tabs>
            </div>
          </main>
        )}
      </AdaptiveSurface>
      )}
      <CreateCustomAgentDialog
        open={createOpen}
        initialRuntimeKind={createRuntimeKind}
        onOpenChange={setCreateOpen}
        onCreate={(agent) => {
          if (!layered) return
          const definition = cloneBuiltInAgent(agent, layered, effective)
          setDraft((current) => ({ ...current, custom_agents: [...(current.custom_agents ?? []), definition] }))
          setActiveSelection(`custom:${agent.id}`)
        }}
      />
    </FeaturePageShell>
  )
}

function activeSelectionIDIsCustom(selection: AgentSelectionID) {
  return selection.startsWith('custom:')
}

function isSubAgentParent(agent: VisibleAgentKey): agent is SubAgentParentKey {
  return agent === 'general' || agent === 'ide' || agent === 'interactive_story'
}

function isCustomizableRuntimeKind(agent: VisibleAgentKey): agent is AgentRuntimeKind {
  return agent === 'general' || agent === 'ide' || agent === 'interactive_story' || agent === 'image'
}
