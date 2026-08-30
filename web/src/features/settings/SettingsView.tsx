import { cloneElement, isValidElement, useEffect, useId, useRef, useState, useCallback } from 'react'
import type { ReactNode } from 'react'
import { ChevronDown, ChevronUp, Download, ExternalLink, Loader2, RefreshCw, Settings as SettingsIcon } from 'lucide-react'
import { useReducedMotionConfig } from 'motion/react'
import type { AnimationPlaybackControls } from 'motion/react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { withErrorLogID } from '@/lib/api-client'
import type { AgentApprovalMode, ImageAPIEndpointSettings, ImageAPIProfileSettings, LabSettings, ModelEndpointSettings, ModelProfileSettings, Settings, ShellEnvironmentMode, UpdateApplyResult, UpdateCheckResult, UpdateInstallProgress, UpdateInstallResult, WebAccessSettings } from './types'
import { applyUpdate, checkForUpdate, GLOBAL_SETTINGS_TARGET, installUpdateStream, revokeAgentApprovalRule } from './api'
import { useLayeredSettingsDraft } from './use-layered-settings-draft'
import { FontPicker } from './FontPicker'
import { getInteractiveTellers } from '@/features/interactive/api'
import type { Teller } from '@/features/interactive/types'
import { DEFAULT_NARRATIVE_STYLE_ID, narrativeStyleName } from '@/features/interactive/narrative-style'
import { InlineErrorNotice } from '@/components/common/inline-error-notice'
import { LoadingState } from '@/components/common/LoadingState'
import { AutosaveStatusIndicator } from '@/components/forms/autosave-status'
import { SettingsFieldRow } from '@/components/forms/settings-field-row'
import { AdaptiveSurface } from '@/components/layout/adaptive-surface'
import { FeaturePageShell } from '@/components/layout/feature-page-shell'
import { MobilePaneTrigger } from '@/components/layout/mobile-pane-trigger'
import { SectionedNavigation } from '@/components/navigation/sectioned-navigation'
import { Button } from '@/components/ui/button'
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { LOCALE_OPTIONS } from '@/i18n'
import { APP_VERSION } from '@/app-version'
import { markAutoUpdateChecked, notifyUpdateCheckResult, shouldRunAutoUpdateCheck } from './update-check-cache'
import { scheduleFrontendReloadAfterUpdate } from './update-reload'
import {
  DEFAULT_MODEL_PROFILE_ID,
  modelEndpointID,
  modelEndpointsWithDefault,
  modelProfileID,
  modelProfilesWithDefault,
} from './model-profiles'
import { ModelProfilesEditor } from './ModelProfilesEditor'
import { DEFAULT_IMAGE_API_PROFILE_ID, imageAPIEndpointID, imageAPIEndpointsWithDefault, imageAPIProfileID, imageAPIProfilesWithDefault } from './image-profiles'
import { ImageAPIProfilesEditor } from './ImageAPIProfilesEditor'
import { ONBOARDING_OPEN_EVENT, SETTINGS_SECTION_EVENT, type SettingsSectionRequest } from '@/features/onboarding/events'
import { TerminalCommandsEditor, terminalCommandsForEditor } from './TerminalCommandsEditor'
import { useAgentApprovalMode } from '@/features/agent-approval/AgentApprovalProvider'
import { AGENT_APPROVAL_MODES } from '@/features/agent-approval/modes'
import { ApprovalRulesEditor } from './ApprovalRulesEditor'
import { scrollSettingsSectionIntoView } from './settings-section-scroll'
import { applyReadingTypographySettings, applyUIFontSize } from './font-variables'
import {
  DEFAULT_READING_FONT_SIZE,
  DEFAULT_UI_FONT_SIZE,
  READING_FONT_SIZE_STEPS,
  UI_FONT_SIZE_STEPS,
} from './font-size-steps'
import { TextSizeControl } from './TextSizeControl'

type SettingsSectionId = 'model' | 'image' | 'paths' | 'access' | 'appearance' | 'updates' | 'labs' | 'agent' | 'terminal' | 'web-access' | 'debug' | 'ide-editor' | 'ide-output' | 'versions' | 'interactive'

const SETTINGS_SECTION_IDS: SettingsSectionId[] = ['model', 'image', 'paths', 'access', 'appearance', 'updates', 'labs', 'agent', 'terminal', 'web-access', 'debug', 'ide-editor', 'ide-output', 'versions', 'interactive']

type SettingsSection = {
  id: SettingsSectionId
  group: string
  title: string
  children: ReactNode
}

const fieldCls = 'nova-field min-h-7 flex-1 rounded-[var(--nova-radius)] border px-2.5 py-1.5 outline-none placeholder:text-[var(--nova-text-faint)] focus:border-[var(--nova-field-focus-border)] focus:bg-[var(--nova-surface-3)]'
const FIELD_INHERIT_VALUE = '__inherit__'
const TRACE_CAPTURE_OPTIONS = [
  { value: 'summary', labelKey: 'settings.debug.traceCaptureSummary' },
  { value: 'debug', labelKey: 'settings.debug.traceCaptureDebug' },
  { value: 'off', labelKey: 'settings.debug.traceCaptureOff' },
] as const
const TRACE_EXPORTER_OPTIONS = [
  { value: 'local', labelKey: 'settings.debug.traceExporterLocal' },
] as const
export function SettingsView({ onClose, visible = true }: { onClose?: () => void; visible?: boolean }) {
  const { t } = useTranslation()
  const reducedMotion = useReducedMotionConfig()
  const approval = useAgentApprovalMode()
  const { layered, draft, setDraft, error, autosaveStatus, autosaveError, saveNow, reload } = useLayeredSettingsDraft({
    target: GLOBAL_SETTINGS_TARGET,
    layer: 'user',
    sourcePrefix: 'settings-view',
  })
  const [availableTellers, setAvailableTellers] = useState<Teller[]>([])
  const [updateStatus, setUpdateStatus] = useState<UpdateCheckResult | null>(null)
  const [updateInstallResult, setUpdateInstallResult] = useState<UpdateInstallResult | null>(null)
  const [updateApplyResult, setUpdateApplyResult] = useState<UpdateApplyResult | null>(null)
  const [updateInstallProgress, setUpdateInstallProgress] = useState<UpdateInstallProgress | null>(null)
  const [checkingUpdate, setCheckingUpdate] = useState(false)
  const [installingUpdate, setInstallingUpdate] = useState(false)
  const [applyingUpdate, setApplyingUpdate] = useState(false)
  const [updateError, setUpdateError] = useState<string | null>(null)
  const [activeSection, setActiveSection] = useState<SettingsSectionId>('appearance')
  const [revokingApprovalRuleID, setRevokingApprovalRuleID] = useState('')
  const [expandedSections, setExpandedSections] = useState<Record<SettingsSectionId, boolean>>({
    model: true,
    image: true,
    paths: true,
    access: true,
    appearance: true,
    updates: true,
    labs: true,
    agent: true,
    terminal: true,
    'web-access': true,
    debug: true,
    'ide-editor': true,
    'ide-output': true,
    versions: true,
    interactive: true,
  })
  const contentRef = useRef<HTMLDivElement | null>(null)
  const sectionRefs = useRef<Partial<Record<SettingsSectionId, HTMLElement | null>>>({})
  const sectionScrollFrameRef = useRef<number | null>(null)
  const sectionScrollAnimationRef = useRef<AnimationPlaybackControls | null>(null)
  const sectionScrollTargetRef = useRef<SettingsSectionId | null>(null)

  useEffect(() => {
    getInteractiveTellers()
      .then((items) => setAvailableTellers(items))
      .catch((e) => console.warn('[settings] Failed to load the director list', e))
  }, [])

  const effective = layered?.effective ?? {}
  const inherited = layered?.inherited?.user ?? {}
  const showDebugSettings = layered?.runtime?.dev_mode === true

  const revokeApprovalRule = useCallback(async (id: string) => {
    if (revokingApprovalRuleID) return
    setRevokingApprovalRuleID(id)
    try {
      await revokeAgentApprovalRule(id)
      await reload()
      toast.success(t('agentApproval.rules.revokeSucceeded'))
    } catch (cause) {
      console.error(`[settings] failed to revoke agent approval rule id=${id}`, cause)
      toast.error(withErrorLogID(t('agentApproval.rules.revokeFailed'), cause))
    } finally {
      setRevokingApprovalRuleID('')
    }
  }, [reload, revokingApprovalRuleID, t])

  const runUpdateCheck = useCallback(async (source: 'auto' | 'manual' = 'manual') => {
    setCheckingUpdate(true)
    setUpdateError(null)
    setUpdateInstallResult(null)
    setUpdateApplyResult(null)
    setUpdateInstallProgress(null)
    try {
      const result = await checkForUpdate()
      setUpdateStatus(result)
      notifyUpdateCheckResult(result)
    } catch (e) {
      setUpdateError((e as Error).message)
    } finally {
      if (source === 'auto') markAutoUpdateChecked()
      setCheckingUpdate(false)
    }
  }, [])

  useEffect(() => {
    if (!layered || effective.update_check_enabled === false || updateStatus || checkingUpdate) return
    if (!shouldRunAutoUpdateCheck()) return
    void runUpdateCheck('auto')
  }, [checkingUpdate, effective.update_check_enabled, layered, runUpdateCheck, updateStatus])

  const runUpdateInstall = useCallback(async () => {
    setInstallingUpdate(true)
    setUpdateError(null)
    setUpdateApplyResult(null)
    setUpdateInstallProgress(null)
    try {
      const stream = await installUpdateStream()
      const reader = stream.getReader()
      while (true) {
        const { done, value } = await reader.read()
        if (done) break
        const data = parseSSEData(value.data)
        if (value.event === 'update_progress') {
          setUpdateInstallProgress(data as unknown as UpdateInstallProgress)
        } else if (value.event === 'update_result') {
          const result = data as unknown as UpdateInstallResult
          setUpdateInstallResult(result)
          setUpdateInstallProgress((prev) => prev ? { ...prev, phase: 'staged', percent: 100 } : { phase: 'staged', percent: 100 })
        } else if (value.event === 'error') {
          throw new Error(readStreamError(data, t))
        }
      }
    } catch (e) {
      setUpdateError((e as Error).message)
    } finally {
      setInstallingUpdate(false)
    }
  }, [t])

  const runUpdateApply = useCallback(async () => {
    setApplyingUpdate(true)
    setUpdateError(null)
    try {
      const result = await applyUpdate()
      setUpdateApplyResult(result)
      scheduleFrontendReloadAfterUpdate(result.version)
    } catch (e) {
      setUpdateError((e as Error).message)
    } finally {
      setApplyingUpdate(false)
    }
  }, [])

  const setField = <K extends keyof Settings>(k: K, v: Settings[K]) =>
    setDraft((d) => ({ ...d, [k]: v }))

  const setWebAccessField = <K extends keyof WebAccessSettings>(key: K, value: WebAccessSettings[K]) =>
    setDraft((current) => ({
      ...current,
      web_access: { ...current.web_access, [key]: value },
    }))

  const setLabField = <K extends keyof LabSettings>(key: K, value: LabSettings[K]) =>
    setDraft((current) => ({
      ...current,
      labs: { ...current.labs, [key]: value },
    }))

  const setModelProfiles = (profiles: ModelProfileSettings[]) => {
    setDraft((d) => ({
      ...d,
      openai_api_key: '',
      openai_base_url: '',
      openai_model: '',
      openai_context_window_tokens: null,
      model_profiles: profiles,
    }))
  }

  const setModelEndpoints = (endpoints: ModelEndpointSettings[]) => {
    setDraft((d) => ({
      ...d,
      openai_api_key: '',
      openai_base_url: '',
      model_endpoints: endpoints,
    }))
  }

  const setDefaultModelProfile = (profileID: string) => {
    setDraft((d) => ({
      ...d,
      agent_models: {
        ...d.agent_models,
        default: { ...d.agent_models?.default, profile_id: profileID },
      },
    }))
  }

  const setImageAPIProfiles = (profiles: ImageAPIProfileSettings[]) => {
    setDraft((d) => ({
      ...d,
      image_api_profiles: profiles,
    }))
  }

  const setImageAPIEndpoints = (endpoints: ImageAPIEndpointSettings[]) => {
    setDraft((d) => ({
      ...d,
      image_api_endpoints: endpoints,
    }))
  }

  const placeholderFor = (k: keyof Settings): string => {
    const v = inherited[k]
    if (v === undefined || v === null || v === '') return t('common.notSet')
    return t('common.inherit', { value: String(v) })
  }

  const webAccessPlaceholderFor = (key: keyof WebAccessSettings): string => {
    const value = inherited.web_access?.[key]
    if (value === undefined || value === null || value === '') return t('common.notSet')
    return t('common.inherit', { value: String(value) })
  }

  const sections: SettingsSection[] = [
    {
      id: 'appearance',
      group: t('settings.group.common'),
      title: t('settings.section.appearance'),
      children: (
        <>
          <LanguageSelect label={t('settings.appearance.language')} value={draft.language}
                          inherited={inherited.language}
                          onChange={(v) => setField('language', v)} />
          <ThemeSelect label={t('settings.appearance.theme')} value={draft.theme}
                       inherited={inherited.theme}
                       onChange={(v) => setField('theme', v)} />
          <MotionIntensitySelect label={t('settings.appearance.motionIntensity')} value={draft.motion_intensity}
                                 inherited={inherited.motion_intensity}
                                 onChange={(v) => setField('motion_intensity', v)} />
          <FieldRow label={t('settings.appearance.uiFont')}>
            <FontPicker value={draft.ui_font_family}
                        inherited={inherited.ui_font_family}
                        allowInherit
                        onValueChange={(v) => setField('ui_font_family', v)} />
          </FieldRow>
          <TextSizeRow
            label={t('settings.appearance.uiFontSize')}
            description={t('settings.appearance.uiFontSizeDescription')}
          >
            <TextSizeControl
              value={draft.ui_font_size ?? effective.ui_font_size ?? DEFAULT_UI_FONT_SIZE}
              steps={UI_FONT_SIZE_STEPS}
              defaultValue={DEFAULT_UI_FONT_SIZE}
              ariaLabel={t('settings.appearance.uiFontSize')}
              disabled={!layered}
              onValueChange={(value) => {
                applyUIFontSize(value)
                setField('ui_font_size', value)
              }}
            />
          </TextSizeRow>
          <FieldRow label={t('settings.appearance.readingFont')}>
            <FontPicker value={draft.reading_font_family}
                        inherited={inherited.reading_font_family}
                        allowInherit
                        onValueChange={(v) => setField('reading_font_family', v)} />
          </FieldRow>
          <TextSizeRow
            label={t('settings.appearance.readingFontSize')}
            description={t('settings.appearance.readingFontSizeDescription')}
          >
            <TextSizeControl
              value={draft.reading_font_size ?? effective.reading_font_size ?? DEFAULT_READING_FONT_SIZE}
              steps={READING_FONT_SIZE_STEPS}
              defaultValue={DEFAULT_READING_FONT_SIZE}
              ariaLabel={t('settings.appearance.readingFontSize')}
              disabled={!layered}
              onValueChange={(value) => {
                applyReadingTypographySettings({
                  readingFont: draft.reading_font_family || effective.reading_font_family,
                  readingFontSize: value,
                })
                setField('reading_font_size', value)
              }}
            />
          </TextSizeRow>
          <FieldRow label={t('settings.appearance.sourceEditorFont')}>
            <FontPicker value={draft.source_editor_font_family}
                        inherited={inherited.source_editor_font_family}
                        allowInherit
                        fallback="mono"
                        onValueChange={(v) => setField('source_editor_font_family', v)} />
          </FieldRow>
          <div data-onboarding-anchor="settings-onboarding" className="flex items-center justify-between gap-3 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-2">
            <div className="min-w-0">
              <div className="text-xs font-medium text-[var(--nova-text)]">{t('settings.onboarding.title')}</div>
              <div className="mt-0.5 text-[11px] leading-4 text-[var(--nova-text-faint)]">{t('settings.onboarding.description')}</div>
            </div>
            <Button
              type="button"
              size="xs"
              variant="ghost"
              className="shrink-0 text-[var(--nova-text-muted)]"
              onClick={() => window.dispatchEvent(new CustomEvent(ONBOARDING_OPEN_EVENT))}
            >
              {t('settings.onboarding.reopen')}
            </Button>
          </div>
        </>
      ),
    },
    {
      id: 'updates',
      group: t('settings.group.common'),
      title: t('settings.section.updates'),
      children: (
        <>
          <BoolTri label={t('settings.updates.autoCheck')} value={draft.update_check_enabled ?? null}
                   inherited={inherited.update_check_enabled}
                   onChange={(v) => setField('update_check_enabled', v)} />
          <UpdatePanel
            status={updateStatus}
            installResult={updateInstallResult}
            applyResult={updateApplyResult}
            installProgress={updateInstallProgress}
            checking={checkingUpdate}
            installing={installingUpdate}
            applying={applyingUpdate}
            error={updateError}
            onCheck={() => void runUpdateCheck()}
            onInstall={() => void runUpdateInstall()}
            onApply={() => void runUpdateApply()}
          />
        </>
      ),
    },
    {
      id: 'model',
      group: t('settings.group.common'),
      title: t('settings.section.model'),
      children: (
        <>
          <ModelProfilesEditor
            endpoints={modelEndpointsForEditor(draft, effective)}
            effectiveEndpoints={modelEndpointsWithDefault(effective)}
            profiles={modelProfilesForEditor(draft, effective)}
            effectiveProfiles={modelProfilesWithDefault(effective)}
            defaultProfileID={draft.agent_models?.default?.profile_id ?? ''}
            effectiveDefaultProfileID={inherited.agent_models?.default?.profile_id || DEFAULT_MODEL_PROFILE_ID}
            onDefaultProfileChange={setDefaultModelProfile}
            onEndpointsChange={setModelEndpoints}
            onProfilesChange={setModelProfiles}
          />
        </>
      ),
    },
    {
      id: 'image',
      group: t('settings.group.common'),
      title: t('settings.section.imageApi'),
      children: (
        <>
          <ImageAPIProfilesEditor
            endpoints={imageAPIEndpointsForEditor(draft, effective)}
            effectiveEndpoints={imageAPIEndpointsWithDefault(effective)}
            profiles={imageAPIProfilesForEditor(draft, effective)}
            effectiveProfiles={imageAPIProfilesWithDefault(effective)}
            defaultProfileID={draft.default_image_api_profile_id ?? ''}
            effectiveDefaultProfileID={inherited.default_image_api_profile_id || DEFAULT_IMAGE_API_PROFILE_ID}
            onDefaultProfileChange={(v) => setField('default_image_api_profile_id', v)}
            onEndpointsChange={setImageAPIEndpoints}
            onProfilesChange={setImageAPIProfiles}
          />
        </>
      ),
    },
    {
      id: 'paths',
      group: t('settings.group.common'),
      title: t('settings.section.paths'),
      children: (
        <>
          <Text label={t('settings.paths.skillsDir')} value={draft.skills_dir} placeholder={placeholderFor('skills_dir')}
                onChange={(v) => setField('skills_dir', v)} />
          <ReadOnly label={t('settings.paths.novaDir')} value={layered?.paths?.denova_dir || layered?.paths?.nova_dir} />
          <ReadOnly label={t('settings.paths.userConfig')} value={layered?.paths?.user_config} />
        </>
      ),
    },
    {
      id: 'access',
      group: t('settings.group.common'),
      title: t('settings.section.access'),
      children: (
        <>
          <BoolTri label={t('settings.access.allowLan')} value={draft.allow_lan_access ?? null}
                   inherited={inherited.allow_lan_access}
                   onChange={(v) => setField('allow_lan_access', v)} />
          <Text label={t('settings.access.username')} value={draft.remote_access_username}
                placeholder={placeholderFor('remote_access_username')}
                onChange={(v) => setField('remote_access_username', v)} />
          <Text label={t('settings.access.password')} value={draft.remote_access_password}
                placeholder={(draft.remote_access_password_set || inherited.remote_access_password_set)
                  ? t('settings.access.passwordSetPlaceholder')
                  : t('settings.access.passwordPlaceholder')}
                onChange={(v) => setField('remote_access_password', v)}
                type="password" />
          <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-2 text-xs leading-5 text-[var(--nova-text-faint)]">
            {t('settings.access.restartHint')}
          </div>
        </>
      ),
    },
    {
      id: 'agent',
      group: t('settings.group.common'),
      title: t('settings.section.agent'),
      children: (
        <>
          <AgentApprovalModeSelect
            value={approval.mode}
            disabled={!approval.initialized || approval.saving}
            onChange={(value) => {
              void approval.setMode(value).then((saved) => {
                if (!saved) toast.error(t('agentApproval.input.changeFailed'))
              })
            }}
          />
          <ApprovalRulesEditor
            rules={draft.agent_approval_rules}
            revokingRuleID={revokingApprovalRuleID}
            onRevoke={(id) => void revokeApprovalRule(id)}
          />
          <Num label={t('settings.agent.maxIteration')} value={draft.max_iteration ?? null}
               placeholder={placeholderFor('max_iteration')}
               onChange={(v) => setField('max_iteration', v)} />
          <Num label={t('settings.agent.modelMaxRetries')} value={draft.model_max_retries ?? null}
               placeholder={placeholderFor('model_max_retries')}
               onChange={(v) => setField('model_max_retries', v)} />
          <Num label={t('settings.agent.idleTimeoutSeconds')} value={draft.agent_idle_timeout_seconds ?? null}
               placeholder={placeholderFor('agent_idle_timeout_seconds')}
               min={0}
               onChange={(v) => setField('agent_idle_timeout_seconds', v)} />
          <Num label={t('settings.agent.toolResultLimitKB')} value={draft.agent_tool_result_limit_kb ?? null}
               placeholder={placeholderFor('agent_tool_result_limit_kb')}
               min={1}
               onChange={(v) => setField('agent_tool_result_limit_kb', v)} />
          <Num label={t('settings.agent.toolParallelism')} value={draft.agent_tool_parallelism ?? null}
               placeholder={placeholderFor('agent_tool_parallelism')}
               min={1}
               max={64}
               onChange={(v) => setField('agent_tool_parallelism', v)} />
          <Num label={t('settings.agent.subAgentParallelism')} value={draft.agent_subagent_parallelism ?? null}
               placeholder={placeholderFor('agent_subagent_parallelism')}
               min={1}
               max={32}
               onChange={(v) => setField('agent_subagent_parallelism', v)} />
          <Num label={t('settings.agent.scriptTimeoutSeconds')} value={draft.agent_script_timeout_seconds ?? null}
               placeholder={placeholderFor('agent_script_timeout_seconds')}
               min={0}
               onChange={(v) => setField('agent_script_timeout_seconds', v)} />
          <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-2 text-xs leading-5 text-[var(--nova-text-faint)]">
            {t('settings.agent.scriptIsolationHint')}
          </div>
          <BoolTri label={t('settings.agent.planModeDefault')} value={draft.plan_mode_default ?? null}
                   inherited={inherited.plan_mode_default}
                   onChange={(v) => setField('plan_mode_default', v)} />
          <Text label={t('settings.agent.writingSkillDefault')} value={draft.writing_skill_default}
                placeholder={placeholderFor('writing_skill_default')}
                onChange={(v) => setField('writing_skill_default', v)} />
          {layered?.runtime?.goos !== 'windows' ? (
            <>
              <ShellEnvironmentSelect
                value={draft.shell_environment_mode}
                inherited={inherited.shell_environment_mode}
                onChange={(v) => setField('shell_environment_mode', v)}
              />
              <Text label={t('settings.agent.shellEnvironmentShell')} value={draft.shell_environment_shell}
                    placeholder={placeholderFor('shell_environment_shell')}
                    onChange={(v) => setField('shell_environment_shell', v)} />
              <Text label={t('settings.agent.bashPath')} value={draft.agent_bash_path}
                    placeholder={placeholderFor('agent_bash_path')}
                    onChange={(v) => setField('agent_bash_path', v)} />
              <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-2 text-xs leading-5 text-[var(--nova-text-faint)]">
                {t('settings.agent.shellEnvironmentHint')}
              </div>
            </>
          ) : null}
        </>
      ),
    },
    {
      id: 'terminal',
      group: t('settings.group.common'),
      title: t('settings.section.terminal'),
      children: (
        <>
          <BoolTri label={t('settings.terminal.enabled')} value={draft.terminal_enabled ?? null}
                   inherited={inherited.terminal_enabled}
                   onChange={(v) => setField('terminal_enabled', v)} />
          <Text label={t('settings.terminal.shell')} value={draft.terminal_shell}
                placeholder={placeholderFor('terminal_shell')}
                onChange={(v) => setField('terminal_shell', v)} />
          <TerminalCommandsEditor
            commands={terminalCommandsForEditor(draft, effective)}
            onChange={(commands) => setField('terminal_commands', commands)}
          />
          <Num label={t('settings.terminal.maxSessions')} value={draft.terminal_max_sessions ?? null}
               placeholder={placeholderFor('terminal_max_sessions')}
               min={1}
               max={64}
               onChange={(v) => setField('terminal_max_sessions', v)} />
          <Num label={t('settings.terminal.scrollbackKB')} value={draft.terminal_scrollback_kb ?? null}
               placeholder={placeholderFor('terminal_scrollback_kb')}
               min={1}
               max={4096}
               onChange={(v) => setField('terminal_scrollback_kb', v)} />
          <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-2 text-xs leading-5 text-[var(--nova-text-faint)]">
            {t('settings.terminal.hint')}
          </div>
        </>
      ),
    },
    {
      id: 'web-access',
      group: t('settings.group.common'),
      title: t('settings.section.webAccess'),
      children: (
        <>
          <Text label={t('settings.webAccess.searxngBaseUrl')} value={draft.web_access?.searxng_base_url}
                placeholder={webAccessPlaceholderFor('searxng_base_url')}
                onChange={(value) => setWebAccessField('searxng_base_url', value)} />
          <Num label={t('settings.webAccess.searchMaxResults')} value={draft.web_access?.search_max_results ?? null}
               placeholder={webAccessPlaceholderFor('search_max_results')}
               min={1}
               max={20}
               onChange={(value) => setWebAccessField('search_max_results', value)} />
          <Num label={t('settings.webAccess.searchProviderTimeoutSeconds')} value={draft.web_access?.search_provider_timeout_seconds ?? null}
               placeholder={webAccessPlaceholderFor('search_provider_timeout_seconds')}
               min={0}
               onChange={(value) => setWebAccessField('search_provider_timeout_seconds', value)} />
          <Num label={t('settings.webAccess.fetchMaxResponseKB')} value={draft.web_access?.fetch_max_response_kb ?? null}
               placeholder={webAccessPlaceholderFor('fetch_max_response_kb')}
               min={1}
               max={65536}
               onChange={(value) => setWebAccessField('fetch_max_response_kb', value)} />
          <Num label={t('settings.webAccess.fetchMaxContentChars')} value={draft.web_access?.fetch_max_content_chars ?? null}
               placeholder={webAccessPlaceholderFor('fetch_max_content_chars')}
               min={1}
               max={262144}
               onChange={(value) => setWebAccessField('fetch_max_content_chars', value)} />
          <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-2 text-xs leading-5 text-[var(--nova-text-faint)]">
            {t('settings.webAccess.hint')}
          </div>
        </>
      ),
    },
    ...(showDebugSettings ? [{
      id: 'debug' as const,
      group: t('settings.group.common'),
      title: t('settings.section.debug'),
      children: (
        <>
          <BoolTri label={t('settings.debug.llmInputLog')} value={draft.llm_input_log_enabled ?? null}
                   inherited={inherited.llm_input_log_enabled}
                   onChange={(v) => setField('llm_input_log_enabled', v)} />
          <TraceCaptureSelect label={t('settings.debug.traceCaptureLevel')} value={draft.trace_capture_level}
                              inherited={inherited.trace_capture_level}
                              onChange={(v) => setField('trace_capture_level', v)} />
          <TraceExporterSelect label={t('settings.debug.traceExporter')} value={draft.trace_exporter}
                               inherited={inherited.trace_exporter}
                               onChange={(v) => setField('trace_exporter', v)} />
          <Num label={t('settings.debug.traceRetentionRuns')} value={draft.trace_retention_runs ?? null}
               placeholder={placeholderFor('trace_retention_runs')}
               min={0}
               onChange={(v) => setField('trace_retention_runs', v)} />
          <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-2 text-xs leading-5 text-[var(--nova-text-faint)]">
            {t('settings.debug.llmInputLogHelp')}
          </div>
          <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-2 text-xs leading-5 text-[var(--nova-text-faint)]">
            {t('settings.debug.traceHelp')}
          </div>
        </>
      ),
    }] : []),
    {
      id: 'ide-editor',
      group: t('settings.group.ide'),
      title: t('settings.section.editor'),
      children: (
        <>
          <BoolTri label={t('settings.ide.autoSave')} value={draft.auto_save_enabled ?? null}
                   inherited={inherited.auto_save_enabled}
                   onChange={(v) => setField('auto_save_enabled', v)} />
          <Num label={t('settings.ide.autoSaveInterval')} value={draft.auto_save_interval_ms ?? null}
               placeholder={placeholderFor('auto_save_interval_ms')}
               onChange={(v) => setField('auto_save_interval_ms', v)} />
          <Text label={t('settings.ide.chapterFilenameFormat')} value={draft.chapter_filename_format}
                placeholder={placeholderFor('chapter_filename_format')}
                onChange={(v) => setField('chapter_filename_format', v)} />
          <Text label={t('settings.ide.volumeDirFormat')} value={draft.volume_dir_format}
                placeholder={placeholderFor('volume_dir_format')}
                onChange={(v) => setField('volume_dir_format', v)} />
          <Num label={t('settings.ide.maxOpenTabs')} value={draft.max_open_tabs ?? null}
               placeholder={placeholderFor('max_open_tabs')}
               onChange={(v) => setField('max_open_tabs', v)} />
          <Num label={t('settings.ide.chapterGroupMin')} value={draft.chapter_group_min ?? null}
               placeholder={placeholderFor('chapter_group_min')}
               onChange={(v) => setField('chapter_group_min', v)} />
          <Num label={t('settings.ide.chapterGroupMax')} value={draft.chapter_group_max ?? null}
               placeholder={placeholderFor('chapter_group_max')}
               onChange={(v) => setField('chapter_group_max', v)} />
          <TellerSelect
            label={t('settings.ide.defaultTeller')}
            value={draft.ide_story_teller_id}
            inherited={inherited.ide_story_teller_id}
            tellers={availableTellers}
            onChange={(v) => setField('ide_story_teller_id', v)}
          />
        </>
      ),
    },
    {
      id: 'versions',
      group: t('settings.group.ide'),
      title: t('settings.section.versions'),
      children: (
        <>
          <BoolTri label={t('settings.versions.timedAuto')} value={draft.version_timed_enabled ?? null}
                   inherited={inherited.version_timed_enabled}
                   onChange={(v) => setField('version_timed_enabled', v)} />
          <Num label={t('settings.versions.timedInterval')} value={draft.version_timed_interval_minutes ?? null}
               placeholder={placeholderFor('version_timed_interval_minutes')}
               min={1}
               onChange={(v) => setField('version_timed_interval_minutes', v)} />
        </>
      ),
    },
    {
      id: 'interactive',
      group: t('settings.group.interactive'),
      title: t('settings.section.interactive'),
      children: (
        <>
          <TellerSelect
            label={t('settings.interactive.defaultTeller')}
            value={draft.interactive_story_teller_id}
            inherited={inherited.interactive_story_teller_id}
            tellers={availableTellers}
            onChange={(v) => setField('interactive_story_teller_id', v)}
          />
          <Num label={t('settings.interactive.lineHeight')} value={draft.interactive_stage_line_height ?? null}
               placeholder={placeholderFor('interactive_stage_line_height')}
               step={0.05}
               onChange={(v) => setField('interactive_stage_line_height', v)} />
        </>
      ),
    },
    {
      id: 'labs',
      group: t('settings.section.labs'),
      title: t('settings.labs.sectionTitle'),
      children: (
        <>
          <div className="mb-3 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-2 text-[11px] leading-4 text-[var(--nova-text-muted)]">
            {t('settings.labs.developerModeHint')}
          </div>
          <BoolTri
            label={t('settings.labs.developerMode')}
            value={draft.labs?.developer_mode ?? null}
            inherited={inherited.labs?.developer_mode}
            onChange={(value) => setLabField('developer_mode', value)}
          />
          {(draft.labs?.developer_mode ?? inherited.labs?.developer_mode ?? false) && (
            <Num
              label={t('settings.labs.continualLearningTrajectoryCap')}
              value={draft.labs?.continual_learning_trajectory_cap ?? null}
              placeholder={String(inherited.labs?.continual_learning_trajectory_cap ?? 100)}
              min={1}
              max={500}
              onChange={(value) => setLabField('continual_learning_trajectory_cap', value)}
            />
          )}
        </>
      ),
    },
  ]

  const cancelSectionScroll = useCallback(() => {
    sectionScrollTargetRef.current = null
    if (sectionScrollFrameRef.current !== null) {
      window.cancelAnimationFrame(sectionScrollFrameRef.current)
      sectionScrollFrameRef.current = null
    }
    sectionScrollAnimationRef.current?.stop()
    sectionScrollAnimationRef.current = null
  }, [])

  useEffect(() => cancelSectionScroll, [cancelSectionScroll])

  const syncActiveSection = useCallback(() => {
    if (sectionScrollTargetRef.current) return
    const current = settingsSectionAtScrollPosition(contentRef.current, sectionRefs.current)
    if (current) setActiveSection(current)
  }, [])

  useEffect(() => {
    if (!visible) {
      cancelSectionScroll()
      return
    }
    if (!layered) return
    const frame = window.requestAnimationFrame(syncActiveSection)
    return () => window.cancelAnimationFrame(frame)
  }, [cancelSectionScroll, layered, syncActiveSection, visible])

  const jumpToSection = useCallback((id: SettingsSectionId) => {
    cancelSectionScroll()
    setActiveSection(id)
    setExpandedSections((prev) => ({ ...prev, [id]: true }))
    sectionScrollTargetRef.current = id
    sectionScrollFrameRef.current = window.requestAnimationFrame(() => {
      sectionScrollFrameRef.current = null
      const container = contentRef.current
      const section = sectionRefs.current[id]
      if (!container || !section) {
        sectionScrollTargetRef.current = null
        return
      }
      sectionScrollAnimationRef.current = scrollSettingsSectionIntoView(container, section, {
        reducedMotion: Boolean(reducedMotion),
        onComplete: () => {
          sectionScrollAnimationRef.current = null
          if (sectionScrollTargetRef.current === id) sectionScrollTargetRef.current = null
        },
      })
    })
  }, [cancelSectionScroll, reducedMotion])

  const toggleSection = (id: SettingsSectionId) => {
    setExpandedSections((prev) => ({ ...prev, [id]: !prev[id] }))
  }

  useEffect(() => {
    const openSection = (event: Event) => {
      const detail = (event as CustomEvent<SettingsSectionRequest>).detail
      const section = detail?.section
      if (!isSettingsSectionId(section)) return
      requestAnimationFrame(() => {
        jumpToSection(section)
      })
    }
    window.addEventListener(SETTINGS_SECTION_EVENT, openSection)
    return () => window.removeEventListener(SETTINGS_SECTION_EVENT, openSection)
  }, [jumpToSection])

  const onContentScroll = () => {
    syncActiveSection()
  }

  const navGroups = sections.reduce<Array<{ group: SettingsSection['group']; items: SettingsSection[] }>>((groups, section) => {
    const last = groups[groups.length - 1]
    if (last?.group === section.group) {
      last.items.push(section)
    } else {
      groups.push({ group: section.group, items: [section] })
    }
    return groups
  }, [])
  const navPanel = (
    <SectionedNavigation
      groups={navGroups.map((group) => ({
        id: group.group,
        title: group.group,
        items: group.items.map((section) => ({ id: section.id, title: section.title })),
      }))}
      activeId={activeSection}
      onSelect={jumpToSection}
    />
  )

  return (
    <FeaturePageShell
      icon={SettingsIcon}
      title={t('settings.title')}
      className="nova-settings-view"
      error={error}
      errorTitle={t('settings.error.save')}
      onClose={onClose ? () => {
        void saveNow().then(() => onClose()).catch(() => undefined)
      } : undefined}
      closeLabel={t('settings.close')}
      onSaveShortcut={() => saveNow().catch(() => undefined)}
      actions={(
        <AutosaveStatusIndicator
          status={autosaveStatus}
          error={autosaveError}
          onRetry={() => saveNow().catch(() => undefined)}
        />
      )}
    >
      {!layered ? (
        <LoadingState label={t('common.loading')} className="min-h-0 flex-1" />
      ) : (
      <AdaptiveSurface
        left={{
          id: 'settings-nav',
          title: t('settings.title'),
          side: 'left',
          icon: <SettingsIcon className="h-4 w-4" />,
          content: navPanel,
          desktopClassName: 'min-h-0 border-r border-[var(--nova-border)]',
          mobileClassName: 'w-[min(86vw,340px)]',
        }}
        className="flex-1 text-xs"
        mainClassName="min-h-0 min-w-0"
        leftResize={{
          layoutKey: 'nova-settings-navigation-layout',
          label: t('layout.resize.sidebar'),
          defaultSize: '224px',
          minSize: '200px',
          maxSize: '36%',
        }}
      >
        {({ openLeft }) => (
          <div
            ref={contentRef}
            data-nova-settings-content="true"
            onScroll={onContentScroll}
            onWheelCapture={cancelSectionScroll}
            onPointerDownCapture={cancelSectionScroll}
            onKeyDownCapture={cancelSectionScroll}
            onTouchStartCapture={cancelSectionScroll}
            className="h-full min-h-0 overflow-y-auto overscroll-contain px-4 py-5 sm:px-6"
          >
            <MobilePaneTrigger
              side="left"
              label={t('workbench.mobile.openSidePanel', { label: t('settings.title') })}
              onClick={openLeft}
              className="mb-3 md:hidden"
            >
              {t('settings.title')}
            </MobilePaneTrigger>
            <div className="mx-auto w-full min-w-0 max-w-5xl">
              {sections.map((section) => (
                <Section
                  key={section.id}
                  id={section.id}
                  ref={(node) => {
                    sectionRefs.current[section.id] = node
                  }}
                  group={section.group}
                  title={section.title}
                  expanded={expandedSections[section.id]}
                  onToggle={() => toggleSection(section.id)}
                >
                  {section.children}
                </Section>
              ))}
            </div>
          </div>
        )}
      </AdaptiveSurface>
      )}
    </FeaturePageShell>
  )
}

function settingsSectionAtScrollPosition(
  container: HTMLElement | null,
  refs: Partial<Record<SettingsSectionId, HTMLElement | null>>,
): SettingsSectionId | null {
  if (!container) return null
  const threshold = container.getBoundingClientRect().top + 72
  let nearestBefore: { id: SettingsSectionId; top: number } | null = null
  let nearestAfter: { id: SettingsSectionId; top: number } | null = null
  for (const [id, node] of Object.entries(refs) as Array<[SettingsSectionId, HTMLElement | null]>) {
    if (!node) continue
    const top = node.getBoundingClientRect().top
    if (top <= threshold && (!nearestBefore || top > nearestBefore.top)) nearestBefore = { id, top }
    if (top > threshold && (!nearestAfter || top < nearestAfter.top)) nearestAfter = { id, top }
  }
  return nearestBefore?.id ?? nearestAfter?.id ?? null
}

export function modelProfilesForEditor(draft: Settings, effective: Settings): ModelProfileSettings[] {
  const localProfiles = draft.model_profiles ?? []
  const hasLocalDefault = localProfiles.some((profile) => modelProfileID(profile) === DEFAULT_MODEL_PROFILE_ID)
  const hasLocalDefaultSelection = Boolean(draft.agent_models?.default?.profile_id?.trim())
  const hasLegacyDefault = Boolean(draft.openai_api_key || draft.openai_base_url || draft.openai_model || draft.openai_context_window_tokens)
  if (hasLocalDefault || hasLocalDefaultSelection || hasLegacyDefault) {
    return preserveDraftOnlyModelProfiles(modelProfilesWithDefault(draft), localProfiles)
  }
  const inherited = modelProfilesWithDefault(effective)
  const localIDs = new Set(localProfiles.map(modelProfileID).filter(Boolean))
  return [
    ...inherited.filter((profile) => !localIDs.has(modelProfileID(profile))),
    ...localProfiles,
  ]
}

export function modelEndpointsForEditor(draft: Settings, effective: Settings): ModelEndpointSettings[] {
  const localEndpoints = draft.model_endpoints ?? []
  const hasLocalDefault = localEndpoints.some((endpoint) => modelEndpointID(endpoint) === 'default')
  const hasLocalDefaultSelection = Boolean(draft.agent_models?.default?.profile_id?.trim())
  const hasLegacyDefault = Boolean(draft.openai_api_key || draft.openai_base_url)
  if (hasLocalDefault || hasLocalDefaultSelection || hasLegacyDefault) return modelEndpointsWithDefault(draft)
  const inherited = modelEndpointsWithDefault(effective)
  const localIDs = new Set(localEndpoints.map(modelEndpointID).filter(Boolean))
  return [
    ...inherited.filter((endpoint) => !localIDs.has(modelEndpointID(endpoint))).map(stripInheritedModelEndpointSecret),
    ...localEndpoints,
  ]
}

function isSettingsSectionId(value: unknown): value is SettingsSectionId {
  return typeof value === 'string' && SETTINGS_SECTION_IDS.includes(value as SettingsSectionId)
}

function preserveDraftOnlyModelProfiles(profiles: ModelProfileSettings[], draftProfiles: ModelProfileSettings[]): ModelProfileSettings[] {
  const draftOnlyProfiles = draftProfiles.filter((profile) => !modelProfileID(profile))
  if (draftOnlyProfiles.length === 0) return profiles
  return [...profiles, ...draftOnlyProfiles]
}

function stripInheritedModelEndpointSecret(endpoint: ModelEndpointSettings): ModelEndpointSettings {
  return { ...endpoint, api_key: '' }
}

export function imageAPIProfilesForEditor(draft: Settings, effective: Settings): ImageAPIProfileSettings[] {
  const localProfiles = draft.image_api_profiles ?? []
  const hasLocalDefault = localProfiles.some((profile) => imageAPIProfileID(profile) === DEFAULT_IMAGE_API_PROFILE_ID)
  const hasLocalDefaultSelection = Boolean(draft.default_image_api_profile_id?.trim())
  if (hasLocalDefault || hasLocalDefaultSelection) {
    return imageAPIProfilesWithDefault(draft)
  }
  const inherited = imageAPIProfilesWithDefault(effective)
  const localIDs = new Set(localProfiles.map(imageAPIProfileID).filter(Boolean))
  return [
    ...inherited.filter((profile) => !localIDs.has(imageAPIProfileID(profile))),
    ...localProfiles,
  ]
}

export function imageAPIEndpointsForEditor(draft: Settings, effective: Settings): ImageAPIEndpointSettings[] {
  const localEndpoints = draft.image_api_endpoints ?? []
  const hasLocalDefault = localEndpoints.some((endpoint) => imageAPIEndpointID(endpoint) === 'default')
  const hasLocalDefaultSelection = Boolean(draft.default_image_api_profile_id?.trim())
  if (hasLocalDefault || hasLocalDefaultSelection) return imageAPIEndpointsWithDefault(draft)
  const inherited = imageAPIEndpointsWithDefault(effective)
  const localIDs = new Set(localEndpoints.map(imageAPIEndpointID).filter(Boolean))
  return [
    ...inherited.filter((endpoint) => !localIDs.has(imageAPIEndpointID(endpoint))).map(stripInheritedImageAPIEndpointSecret),
    ...localEndpoints,
  ]
}

function stripInheritedImageAPIEndpointSecret(endpoint: ImageAPIEndpointSettings): ImageAPIEndpointSettings {
  return { ...endpoint, api_key: '' }
}

function Section({
  ref,
  id,
  group,
  title,
  expanded,
  onToggle,
  children,
}: {
  ref?: (node: HTMLElement | null) => void
  id: SettingsSectionId
  group: string
  title: string
  expanded: boolean
  onToggle: () => void
  children: ReactNode
}) {
  return (
    <section ref={ref} data-onboarding-anchor={id === 'model' ? 'settings-model' : undefined} className="scroll-mt-4 border-b border-[var(--nova-border)] py-4 first:pt-0 last:border-b-0">
      <button
        type="button"
        onClick={onToggle}
        className="nova-nav-item mb-2 flex w-full items-center justify-between rounded-[var(--nova-radius)] px-1.5 py-1 text-left"
        aria-expanded={expanded}
      >
        <span className="min-w-0">
          <span className="mr-2 text-[11px] text-[var(--nova-text-faint)]">{group}</span>
          <span className="font-medium text-[var(--nova-text)]">{title}</span>
        </span>
        {expanded ? (
          <ChevronUp className="h-3.5 w-3.5 text-[var(--nova-text-faint)]" />
        ) : (
          <ChevronDown className="h-3.5 w-3.5 text-[var(--nova-text-faint)]" />
        )}
      </button>
      {expanded && (
        <div className="nova-settings-section-card flex flex-col gap-2 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] p-3">{children}</div>
      )}
    </section>
  )
}

export function UpdatePanel({
  status,
  installResult,
  applyResult,
  installProgress,
  checking,
  installing,
  applying,
  error,
  onCheck,
  onInstall,
  onApply,
}: {
  status: UpdateCheckResult | null
  installResult: UpdateInstallResult | null
  applyResult: UpdateApplyResult | null
  installProgress: UpdateInstallProgress | null
  checking: boolean
  installing: boolean
  applying: boolean
  error: string | null
  onCheck: () => void
  onInstall: () => void
  onApply: () => void
}) {
  const { t } = useTranslation()
  const releaseDate = status?.published_at ? new Date(status.published_at).toLocaleString() : ''
  const applyReady = Boolean(installResult?.apply_ready)
  const restarting = Boolean(applyResult)
  const installDisabled = installing || checking || applying || restarting || !status?.can_install || applyReady
  const applyDisabled = checking || installing || applying || restarting || !applyReady
  const progressPercent = clampPercent(installProgress?.percent ?? 0)
  const progressLabel = installProgress ? updatePhaseLabel(installProgress.phase, t) : ''
  return (
    <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-3">
      <div className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
        <div className="min-w-0 flex flex-col gap-1">
          <div className="flex flex-wrap items-center gap-2">
            <span className="font-medium text-[var(--nova-text)]">{status ? updateStatusLabel(status, t) : t('settings.updates.notChecked')}</span>
            {status?.update_available && (
              <span className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-active)] px-1.5 py-0.5 text-[11px] text-[var(--nova-text)]">
                {t('settings.updates.available')}
              </span>
            )}
          </div>
          <div className="grid gap-1 text-[var(--nova-text-faint)] sm:grid-cols-2">
            <span>{t('settings.updates.currentVersion', { version: status?.current_version || APP_VERSION })}</span>
            <span>{t('settings.updates.latestVersion', { version: status?.latest_version || t('common.notSet') })}</span>
            <span>{t('settings.updates.platform', { platform: status?.platform || t('common.notSet') })}</span>
            <span>{t('settings.updates.publishedAt', { time: releaseDate || t('common.notSet') })}</span>
          </div>
          {status?.asset && (
            <div className="truncate text-[var(--nova-text-faint)]">
              {t('settings.updates.asset', { name: status.asset.name, size: formatBytes(status.asset.size) })}
            </div>
          )}
          {installProgress && (
            <div className="mt-2 flex flex-col gap-1.5 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-2.5 py-2">
              <div className="flex items-center justify-between gap-3 text-[var(--nova-text-muted)]">
                <span>{progressLabel}</span>
                <span>{t('settings.updates.progressPercent', { percent: Math.round(progressPercent) })}</span>
              </div>
              <div className="h-1.5 overflow-hidden rounded-full bg-[var(--nova-surface-3)]" aria-label={t('settings.updates.progressAria')}>
                <div
                  className="h-full rounded-full bg-[var(--nova-text)] transition-[width] duration-200"
                  style={{ width: `${progressPercent}%` }}
                />
              </div>
              <div className="flex flex-col gap-1 text-[11px] text-[var(--nova-text-faint)] sm:flex-row sm:items-center sm:justify-between">
                <span>{t('settings.updates.downloaded', {
                  downloaded: formatBytes(installProgress.downloaded_bytes ?? 0),
                  total: installProgress.total_bytes ? formatBytes(installProgress.total_bytes) : t('common.notSet'),
                })}</span>
                {installProgress.archive_path && (
                  <span className="max-w-full truncate">{t('settings.updates.localPackage', { path: installProgress.archive_path })}</span>
                )}
              </div>
            </div>
          )}
          {installResult?.apply_ready && (
            <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-2.5 py-1.5 text-[var(--nova-text-muted)]">
              {t('settings.updates.stagedRestart')}
            </div>
          )}
          {applyResult && (
            <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-2.5 py-1.5 text-[var(--nova-text-muted)]">
              {t('settings.updates.applyingRestart')}
            </div>
          )}
          {error && <InlineErrorNotice className="mt-2" message={error} title={t('settings.updates.error')} />}
        </div>
        <div className="flex shrink-0 flex-wrap gap-2">
          {status?.release_url && (
            <a
              href={status.release_url}
              target="_blank"
              rel="noreferrer"
              className="nova-nav-item inline-flex items-center gap-1.5 rounded-[var(--nova-radius)] border border-[var(--nova-border)] px-2.5 py-1 text-[var(--nova-text)]"
            >
              <ExternalLink className="h-3.5 w-3.5" />
              {t('settings.updates.openRelease')}
            </a>
          )}
          <button
            type="button"
            onClick={onCheck}
            disabled={checking || installing || applying || restarting}
            className="nova-nav-item inline-flex items-center gap-1.5 rounded-[var(--nova-radius)] border border-[var(--nova-border)] px-2.5 py-1 text-[var(--nova-text)] disabled:opacity-50"
          >
            <RefreshCw className={`h-3.5 w-3.5 ${checking ? 'animate-spin' : ''}`} />
            {checking ? t('settings.updates.checking') : t('settings.updates.check')}
          </button>
          <button
            type="button"
            onClick={onInstall}
            disabled={installDisabled}
            className="nova-nav-item inline-flex items-center gap-1.5 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-active)] px-2.5 py-1 text-[var(--nova-text)] disabled:opacity-50"
          >
            {installing ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Download className="h-3.5 w-3.5" />}
            {installing ? t('settings.updates.installing') : t('settings.updates.install')}
          </button>
          {applyReady && (
            <button
              type="button"
              onClick={onApply}
              disabled={applyDisabled}
              className="nova-nav-item inline-flex items-center gap-1.5 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-active)] px-2.5 py-1 text-[var(--nova-text)] disabled:opacity-50"
            >
              {applying ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <RefreshCw className="h-3.5 w-3.5" />}
              {applying ? t('settings.updates.applying') : t('settings.updates.apply')}
            </button>
          )}
        </div>
      </div>
    </div>
  )
}

function updateStatusLabel(status: UpdateCheckResult, t: (key: string, args?: Record<string, unknown>) => string) {
  if (status.update_available) return t('settings.updates.updateAvailableTitle')
  return t('settings.updates.upToDateTitle')
}

function updatePhaseLabel(phase: string, t: (key: string, args?: Record<string, unknown>) => string) {
  switch (phase) {
    case 'checking':
      return t('settings.updates.phase.checking')
    case 'downloading':
      return t('settings.updates.phase.downloading')
    case 'verifying':
      return t('settings.updates.phase.verifying')
    case 'extracting':
      return t('settings.updates.phase.extracting')
    case 'replacing':
      return t('settings.updates.phase.replacing')
    case 'staging':
      return t('settings.updates.phase.staging')
    case 'staged':
      return t('settings.updates.phase.staged')
    case 'installed':
      return t('settings.updates.phase.installed')
    default:
      return t('settings.updates.phase.running')
  }
}

function parseSSEData(data: string): Record<string, unknown> {
  try {
    return JSON.parse(data) as Record<string, unknown>
  } catch {
    return {}
  }
}

function readStreamError(data: Record<string, unknown>, t: (key: string) => string) {
  return typeof data.message === 'string' && data.message ? data.message : t('settings.updates.error')
}

function clampPercent(value: number) {
  if (!Number.isFinite(value)) return 0
  return Math.min(100, Math.max(0, value))
}

function formatBytes(value: number) {
  if (!Number.isFinite(value) || value <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB']
  let size = value
  let index = 0
  while (size >= 1024 && index < units.length - 1) {
    size /= 1024
    index += 1
  }
  return `${size.toFixed(index === 0 ? 0 : 1)} ${units[index]}`
}

function FieldRow({ label, children }: { label: string; children: ReactNode }) {
  const generatedID = useId()
  const childID = isValidElement<{ id?: string }>(children) ? children.props.id : undefined
  const controlID = childID || generatedID
  const control = isValidElement<{ id?: string }>(children)
    ? cloneElement(children, { id: controlID })
    : children
  return (
    <SettingsFieldRow
      title={label}
      htmlFor={controlID}
      className="nova-settings-row rounded-md border-0 bg-transparent px-2 py-1.5"
      contentClassName="sm:w-44 sm:flex-none"
      controlClassName="flex-1"
    >
      {control}
    </SettingsFieldRow>
  )
}

function ValueRow({ label, children }: { label: string; children: ReactNode }) {
  return (
    <SettingsFieldRow
      title={label}
      className="nova-settings-row rounded-md border-0 bg-transparent px-2 py-1.5"
      contentClassName="sm:w-44 sm:flex-none"
      controlClassName="flex-1"
    >
      {children}
    </SettingsFieldRow>
  )
}

function TextSizeRow({ label, description, children }: { label: string; description: string; children: ReactNode }) {
  return (
    <SettingsFieldRow
      title={label}
      description={description}
      className="nova-settings-row rounded-md border-0 bg-transparent px-2 py-2"
      contentClassName="sm:w-44 sm:flex-none"
      controlClassName="flex-1 sm:max-w-md"
    >
      {children}
    </SettingsFieldRow>
  )
}

function ReadOnly({ label, value }: { label: string; value?: string }) {
  const { t } = useTranslation()
  return (
    <ValueRow label={label}>
      <code className="min-h-7 flex-1 truncate rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2.5 py-1.5 text-[var(--nova-text-muted)]">
        {value || t('common.notSet')}
      </code>
    </ValueRow>
  )
}

function Text({ label, value, placeholder, type = 'text', disabled, onChange }: {
  label: string; value?: string; placeholder?: string; type?: string; disabled?: boolean
  onChange: (v: string) => void
}) {
  return (
    <FieldRow label={label}>
      <input
        type={type}
        value={value ?? ''}
        placeholder={placeholder}
        disabled={disabled}
        onChange={(e) => onChange(e.target.value)}
        className={`${fieldCls} disabled:opacity-50`}
      />
    </FieldRow>
  )
}

function Num({ label, value, placeholder, step = 1, min, max, onChange }: {
  label: string; value: number | null; placeholder?: string
  step?: number
  min?: number
  max?: number
  onChange: (v: number | null) => void
}) {
  return (
    <FieldRow label={label}>
      <input
        type="number"
        step={step}
        min={min}
        max={max}
        value={value ?? ''}
        placeholder={placeholder}
        onChange={(e) => {
          const raw = e.target.value
          onChange(raw === '' ? null : Number(raw))
        }}
        className={fieldCls}
      />
    </FieldRow>
  )
}

function BoolTri({ label, value, inherited, onChange }: {
  label: string; value: boolean | null; inherited?: boolean | null
  onChange: (v: boolean | null) => void
}) {
  const { t } = useTranslation()
  const inheritedLabel = inherited === null || inherited === undefined ? t('common.notSet') : String(inherited)
  const selectValue = value === null ? FIELD_INHERIT_VALUE : String(value)
  return (
    <FieldRow label={label}>
      <Select value={selectValue} onValueChange={(v) => onChange(v === FIELD_INHERIT_VALUE ? null : v === 'true')}>
        <SelectTrigger size="sm" className="w-full">
          <SelectValue />
        </SelectTrigger>
        <SelectContent className="nova-panel border text-[var(--nova-text)]">
          <SelectGroup>
            <SelectItem value={FIELD_INHERIT_VALUE}>{t('common.inherit', { value: inheritedLabel })}</SelectItem>
            <SelectItem value="true">{t('settings.bool.true')}</SelectItem>
            <SelectItem value="false">{t('settings.bool.false')}</SelectItem>
          </SelectGroup>
        </SelectContent>
      </Select>
    </FieldRow>
  )
}

function AgentApprovalModeSelect({ value, disabled, onChange }: {
  value: AgentApprovalMode
  disabled?: boolean
  onChange: (value: AgentApprovalMode) => void
}) {
  const { t } = useTranslation()
  return (
    <FieldRow label={t('settings.agent.approvalMode')}>
      <div className="grid gap-1.5">
        <Select value={value} disabled={disabled} onValueChange={(next) => onChange(next as AgentApprovalMode)}>
          <SelectTrigger size="sm" className="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent className="nova-panel border text-[var(--nova-text)]">
            {AGENT_APPROVAL_MODES.map((mode) => (
              <SelectItem key={mode} value={mode}>
                {t(`agentApproval.mode.${mode}.label`)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <span className="text-[11px] leading-4 text-[var(--nova-text-faint)]">{t(`agentApproval.mode.${value}.description`)}</span>
      </div>
    </FieldRow>
  )
}

function ShellEnvironmentSelect({ value, inherited, onChange }: {
  value?: ShellEnvironmentMode
  inherited?: ShellEnvironmentMode
  onChange: (value: ShellEnvironmentMode | undefined) => void
}) {
  const { t } = useTranslation()
  const inheritedValue = inherited || 'auto'
  return (
    <FieldRow label={t('settings.agent.shellEnvironmentMode')}>
      <Select value={value || FIELD_INHERIT_VALUE} onValueChange={(next) => onChange(next === FIELD_INHERIT_VALUE ? undefined : next as ShellEnvironmentMode)}>
        <SelectTrigger size="sm" className="w-full">
          <SelectValue />
        </SelectTrigger>
        <SelectContent className="nova-panel border text-[var(--nova-text)]">
          <SelectItem value={FIELD_INHERIT_VALUE}>{t('common.inherit', { value: t(`settings.agent.shellEnvironment.${inheritedValue}`) })}</SelectItem>
          <SelectItem value="auto">{t('settings.agent.shellEnvironment.auto')}</SelectItem>
          <SelectItem value="process">{t('settings.agent.shellEnvironment.process')}</SelectItem>
        </SelectContent>
      </Select>
    </FieldRow>
  )
}

function TraceCaptureSelect({ label, value, inherited, onChange }: {
  label: string
  value?: string
  inherited?: string
  onChange: (v: string) => void
}) {
  const { t } = useTranslation()
  const inheritedValue = inherited || 'summary'
  const inheritedLabel = t(TRACE_CAPTURE_OPTIONS.find((option) => option.value === inheritedValue)?.labelKey || 'settings.debug.traceCaptureSummary')
  return (
    <FieldRow label={label}>
      <Select value={value || FIELD_INHERIT_VALUE} onValueChange={(v) => onChange(v === FIELD_INHERIT_VALUE ? '' : v)}>
        <SelectTrigger size="sm" className="w-full">
          <SelectValue />
        </SelectTrigger>
        <SelectContent className="nova-panel border text-[var(--nova-text)]">
          <SelectGroup>
            <SelectItem value={FIELD_INHERIT_VALUE}>{t('common.inherit', { value: inheritedLabel })}</SelectItem>
            {TRACE_CAPTURE_OPTIONS.map((option) => (
              <SelectItem key={option.value} value={option.value}>{t(option.labelKey)}</SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
    </FieldRow>
  )
}

function TraceExporterSelect({ label, value, inherited, onChange }: {
  label: string
  value?: string
  inherited?: string
  onChange: (v: string) => void
}) {
  const { t } = useTranslation()
  const inheritedValue = TRACE_EXPORTER_OPTIONS.some((option) => option.value === inherited) ? inherited || 'local' : 'local'
  const isValidValue = TRACE_EXPORTER_OPTIONS.some((option) => option.value === value)
  const selectValue = isValidValue ? value || FIELD_INHERIT_VALUE : FIELD_INHERIT_VALUE
  const inheritedLabel = t(TRACE_EXPORTER_OPTIONS.find((option) => option.value === inheritedValue)?.labelKey || 'settings.debug.traceExporterLocal')
  return (
    <FieldRow label={label}>
      <Select value={selectValue} onValueChange={(v) => onChange(v === FIELD_INHERIT_VALUE ? '' : v)}>
        <SelectTrigger size="sm" className="w-full">
          <SelectValue />
        </SelectTrigger>
        <SelectContent className="nova-panel border text-[var(--nova-text)]">
          <SelectGroup>
            <SelectItem value={FIELD_INHERIT_VALUE}>{t('common.inherit', { value: inheritedLabel })}</SelectItem>
            {TRACE_EXPORTER_OPTIONS.map((option) => (
              <SelectItem key={option.value} value={option.value}>{t(option.labelKey)}</SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
    </FieldRow>
  )
}

function LanguageSelect({ label, value, inherited, onChange }: {
  label: string
  value?: string
  inherited?: string
  onChange: (v: string) => void
}) {
  const { t } = useTranslation()
  const inheritedLabel = t(LOCALE_OPTIONS.find((option) => option.value === (inherited || 'auto'))?.labelKey || 'locale.auto')
  return (
    <FieldRow label={label}>
      <Select value={value || FIELD_INHERIT_VALUE} onValueChange={(v) => onChange(v === FIELD_INHERIT_VALUE ? '' : v)}>
        <SelectTrigger size="sm" className="w-full">
          <SelectValue />
        </SelectTrigger>
        <SelectContent className="nova-panel border text-[var(--nova-text)]">
          <SelectGroup>
            <SelectItem value={FIELD_INHERIT_VALUE}>{t('common.inherit', { value: inheritedLabel })}</SelectItem>
            {LOCALE_OPTIONS.map((option) => (
              <SelectItem key={option.value} value={option.value}>{t(option.labelKey)}</SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
    </FieldRow>
  )
}

const THEME_OPTIONS = [
  { value: 'dark', labelKey: 'settings.theme.dark' },
  { value: 'light', labelKey: 'settings.theme.light' },
  { value: 'system', labelKey: 'settings.theme.system' },
] as const

const MOTION_INTENSITY_OPTIONS = [
  { value: 'system', labelKey: 'settings.motion.system' },
  { value: 'full', labelKey: 'settings.motion.full' },
  { value: 'reduced', labelKey: 'settings.motion.reduced' },
  { value: 'off', labelKey: 'settings.motion.off' },
] as const

function ThemeSelect({ label, value, inherited, onChange }: {
  label: string
  value?: string
  inherited?: string
  onChange: (v: string) => void
}) {
  const { t } = useTranslation()
  const inheritedValue = inherited || 'dark'
  const inheritedLabel = t(THEME_OPTIONS.find((option) => option.value === inheritedValue)?.labelKey || 'settings.theme.dark')
  return (
    <FieldRow label={label}>
      <Select value={value || FIELD_INHERIT_VALUE} onValueChange={(v) => onChange(v === FIELD_INHERIT_VALUE ? '' : v)}>
        <SelectTrigger size="sm" className="w-full">
          <SelectValue />
        </SelectTrigger>
        <SelectContent className="nova-panel border text-[var(--nova-text)]">
          <SelectGroup>
            <SelectItem value={FIELD_INHERIT_VALUE}>{t('common.inherit', { value: inheritedLabel })}</SelectItem>
            {THEME_OPTIONS.map((option) => (
              <SelectItem key={option.value} value={option.value}>{t(option.labelKey)}</SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
    </FieldRow>
  )
}

function MotionIntensitySelect({ label, value, inherited, onChange }: {
  label: string
  value?: string
  inherited?: string
  onChange: (v: string) => void
}) {
  const { t } = useTranslation()
  const inheritedValue = inherited || 'system'
  const inheritedLabel = t(MOTION_INTENSITY_OPTIONS.find((option) => option.value === inheritedValue)?.labelKey || 'settings.motion.system')
  return (
    <FieldRow label={label}>
      <Select value={value || FIELD_INHERIT_VALUE} onValueChange={(v) => onChange(v === FIELD_INHERIT_VALUE ? '' : v)}>
        <SelectTrigger size="sm" className="w-full">
          <SelectValue />
        </SelectTrigger>
        <SelectContent className="nova-panel border text-[var(--nova-text)]">
          <SelectGroup>
            <SelectItem value={FIELD_INHERIT_VALUE}>{t('common.inherit', { value: inheritedLabel })}</SelectItem>
            {MOTION_INTENSITY_OPTIONS.map((option) => (
              <SelectItem key={option.value} value={option.value}>{t(option.labelKey)}</SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
    </FieldRow>
  )
}

function TellerSelect({ label, value, inherited, tellers, onChange }: {
  label: string
  value?: string
  inherited?: string
  tellers: Teller[]
  onChange: (v: string) => void
}) {
  const { t } = useTranslation()
  const inheritedTeller = tellers.find((teller) => teller.id === inherited)
  const inheritedName = inheritedTeller ? narrativeStyleName(inheritedTeller, t) : inherited || DEFAULT_NARRATIVE_STYLE_ID
  return (
    <FieldRow label={label}>
      <Select value={value || FIELD_INHERIT_VALUE} onValueChange={(v) => onChange(v === FIELD_INHERIT_VALUE ? '' : v)}>
        <SelectTrigger size="sm" className="w-full">
          <SelectValue />
        </SelectTrigger>
        <SelectContent className="nova-panel border text-[var(--nova-text)]">
          <SelectGroup>
            <SelectItem value={FIELD_INHERIT_VALUE}>{t('common.inherit', { value: inheritedName })}</SelectItem>
            {tellers.map((teller) => (
              <SelectItem key={teller.id} value={teller.id}>{narrativeStyleName(teller, t)}</SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
    </FieldRow>
  )
}
