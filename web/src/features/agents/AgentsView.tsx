import { useCallback, useEffect, useId, useMemo, useState } from 'react'
import { Bot } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { ConfigManagerChat } from '@/components/Chat/ConfigManagerChat'
import { AutosaveStatusIndicator } from '@/components/forms/autosave-status'
import { SettingsFieldRow } from '@/components/forms/settings-field-row'
import { AdaptiveSurface } from '@/components/layout/adaptive-surface'
import { FeaturePageShell } from '@/components/layout/feature-page-shell'
import { MobilePaneTrigger } from '@/components/layout/mobile-pane-trigger'
import { SidebarVisibilityToggle } from '@/components/layout/sidebar-visibility-toggle'
import { SectionedNavigation } from '@/components/navigation/sectioned-navigation'
import type { SectionedNavigationGroup } from '@/components/navigation/sectioned-navigation'
import { Button } from '@/components/ui/button'
import { LoadingState } from '@/components/common/LoadingState'
import { Input } from '@/components/ui/input'
import type { AgentContextOverride, AgentModelOverride, AgentSkillOverride, AgentToolOverride, ImageAPIProfileSettings, LayeredSettings, ModelProfileSettings, Settings, SettingsLayer } from '@/features/settings/types'
import { modelProfileID, modelProfileLabel, modelProfilesWithDefault } from '@/features/settings/model-profiles'
import { imageAPIProfileID, imageAPIProfileLabel, imageAPIProfilesWithDefault } from '@/features/settings/image-profiles'
import { useLayeredSettingsDraft } from '@/features/settings/use-layered-settings-draft'
import { getSkills, resourceTargetKey } from '@/lib/api'
import type { ResourceTarget, SkillSummary } from '@/lib/api'
import { AgentRuntimeContextSection } from './AgentRuntimeContextSection'
import { AgentBuiltInCapabilitySection, AgentContextSection, AgentImageModelSection, AgentModelOnlySection, AgentModelSection, AgentSkillSection, AgentToolSection, mergeAgentModelOverride } from './agent-configuration-sections'
import { AGENTS, toolDefinitionsFromManifest } from './agent-registry'
import type { AgentViewDefinition, ToolKey, VisibleAgentKey } from './agent-registry'

const tabCls = 'nova-nav-item rounded-[var(--nova-radius)] px-2.5 py-1 text-xs'

export function AgentsView({ target, onClose }: { target: ResourceTarget; onClose?: () => void }) {
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
  const { layered, draft, setDraft, error, autosaveStatus, autosaveError, reload, notifyUpdated, saveNow } = useLayeredSettingsDraft({
    target: resourceTarget,
    layer: activeLayer,
    sourcePrefix: 'agents-view',
  })
  const [activeAgent, setActiveAgent] = useState<VisibleAgentKey>('ide')
  const [skills, setSkills] = useState<SkillSummary[]>([])
  const [agentChatOpen, setAgentChatOpen] = useState(false)
  const [sidebarVisible, setSidebarVisible] = useState(true)

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
  const selected = AGENTS.find((agent) => agent.key === activeAgent) ?? AGENTS[0]
  const profileOptions = useMemo(() => buildProfileOptions(draft, effective, t), [draft, effective, t])
  const imageProfileOptions = useMemo(() => buildImageProfileOptions(draft, effective, t), [draft, effective, t])
  const inheritedImageProfileID = resolveInheritedImageProfileID(layered, activeLayer)
  const modelValue = draft.agent_models?.[activeAgent] ?? {}
  const inheritedModel = mergeAgentModelOverride(effective.agent_models?.default ?? {}, effective.agent_models?.[activeAgent] ?? {})
  const toolValue = draft.agent_tools?.[activeAgent] ?? {}
  const resolvedToolManifest = layered?.resolved_agent_tool_manifests?.[activeAgent]
  const toolRows = useMemo(() => toolDefinitionsFromManifest(resolvedToolManifest), [resolvedToolManifest])
  const skillsAllowed = Boolean(toolValue.skills ?? toolRows.find((tool) => tool.key === 'skills')?.allowed ?? false)
  const skillValue = draft.agent_skills?.[activeAgent] ?? {}
  const contextValue = draft.agent_context?.[activeAgent] ?? {}
  const resolvedContext = layered?.resolved_agent_contexts?.[activeAgent]
  const inheritedToolParallelism = resolveInheritedToolParallelism(layered, activeLayer)
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
        notifyUpdated(activeLayer)
      })
      .catch(() => undefined)
  }, [activeLayer, notifyUpdated, reload, saveNow])

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

  const setToolParallelism = (value: number | null) => {
    setDraft((current) => ({ ...current, agent_tool_parallelism: value }))
  }

  const setImageProfile = (profileID: string) => {
    setDraft((current) => ({ ...current, default_image_api_profile_id: profileID }))
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
          content: <div className="h-full min-h-0 overflow-y-auto bg-[var(--nova-surface-2)] p-3"><AgentList active={activeAgent} onSelect={setActiveAgent} /></div>,
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
                <span className="min-w-0 truncate text-[11px] text-[var(--nova-text-muted)]">{t(selected.titleKey)}</span>
                {agentAvailable && agentChatOpen && (
                  <MobilePaneTrigger side="right" label={t('workbench.mobile.openSidePanel', { label: t('agents.configAgent.title') })} onClick={openRight} className="ml-auto" />
                )}
              </div>
            )}
            <div className="mx-auto flex w-full min-w-0 max-w-5xl flex-col gap-5 px-4 py-5 sm:px-6">
              <AgentHeader agent={selected} />
              <AgentToolSchedulingSection
                value={draft.agent_tool_parallelism ?? null}
                inherited={inheritedToolParallelism}
                onChange={setToolParallelism}
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
              {activeAgent === 'image' && (
                <AgentImageModelSection
                  value={draft.default_image_api_profile_id ?? ''}
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
              <AgentContextSection agent={selected.key} effective={effective} resolved={resolvedContext} />
            </div>
          </main>
        )}
      </AdaptiveSurface>
      )}
    </FeaturePageShell>
  )
}

function AgentToolSchedulingSection({
  value,
  inherited,
  onChange,
}: {
  value: number | null
  inherited: number
  onChange: (value: number | null) => void
}) {
  const { t } = useTranslation()
  const inputID = useId()
  return (
    <section className="border-b border-[var(--nova-border)] pb-5">
      <h2 className="mb-3 text-xs font-semibold uppercase tracking-[0.12em] text-[var(--nova-text-muted)]">
        {t('agents.section.toolScheduling')}
      </h2>
      <SettingsFieldRow
        htmlFor={inputID}
        title={t('settings.agent.toolParallelism')}
        description={t('agents.tool.parallelismNote')}
        meta={value === null ? <span className="text-[10px] text-[var(--nova-text-faint)]">{t('common.inherit', { value: inherited })}</span> : undefined}
        controlClassName="sm:w-36"
      >
        <Input
          id={inputID}
          type="number"
          min={1}
          max={64}
          value={value ?? ''}
          placeholder={String(inherited)}
          aria-label={t('settings.agent.toolParallelism')}
          onChange={(event) => {
            if (event.target.value === '') {
              onChange(null)
              return
            }
            const parsed = Number(event.target.value)
            if (Number.isFinite(parsed)) onChange(Math.min(64, Math.max(1, Math.trunc(parsed))))
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

function AgentList({ active, onSelect }: { active: VisibleAgentKey; onSelect: (agent: VisibleAgentKey) => void }) {
  const { t } = useTranslation()
  const groups = AGENTS.reduce<Array<{ group: string; agents: typeof AGENTS }>>((acc, agent) => {
    const last = acc[acc.length - 1]
    if (last?.group === agent.groupKey) last.agents.push(agent)
    else acc.push({ group: agent.groupKey, agents: [agent] })
    return acc
  }, [])

  const navigationGroups: SectionedNavigationGroup<VisibleAgentKey>[] = groups.map((group, index) => ({
    id: `${group.group}:${index}`,
    title: t(group.group),
    items: group.agents.map((agent) => ({
      id: agent.key,
      title: t(agent.titleKey),
      description: t(agent.subtitleKey),
      icon: agent.icon,
    })),
  }))
  return (
    <SectionedNavigation
      groups={navigationGroups}
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
