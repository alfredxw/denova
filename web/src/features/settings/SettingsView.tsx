import { EMPTY_SPEECH_SETTINGS, SpeechSettingsEditor } from './SpeechSettingsEditor'
import { useEffect, useState, useCallback } from 'react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from '@/lib/toast'
import { withErrorLogID } from '@/lib/api-client'
import type { ImageAPIEndpointSettings, ImageAPIProfileSettings, LabSettings, ModelEndpointSettings, ModelProfileSettings, Settings, WebAccessSettings } from './types'
import { GLOBAL_SETTINGS_TARGET, revokeAgentApprovalRule } from './api'
import { useLayeredSettingsDraft } from './use-layered-settings-draft'
import { FontPicker } from './FontPicker'
import { getInteractiveTellers } from '@/features/interactive/api'
import type { Teller } from '@/features/interactive/types'
import { LoadingState } from '@/components/common/LoadingState'
import { AutosaveStatusIndicator } from '@/components/forms/autosave-status'
import { SettingsFieldRow } from '@/components/forms/settings-field-row'
import { SettingsPageFrame } from './SettingsPageFrame'
import { Button } from '@/components/ui/button'
import { DEFAULT_MODEL_PROFILE_ID, modelEndpointsWithDefault, modelProfilesWithDefault } from './model-profiles'
import { ModelProfilesEditor } from './ModelProfilesEditor'
import { DEFAULT_IMAGE_API_PROFILE_ID, imageAPIEndpointsWithDefault, imageAPIProfilesWithDefault } from './image-profiles'
import { ImageAPIProfilesEditor } from './ImageAPIProfilesEditor'
import { ONBOARDING_OPEN_EVENT } from '@/features/onboarding/events'
import { TerminalCommandsEditor, terminalCommandsForEditor } from './TerminalCommandsEditor'
import { TerminalShellField } from './TerminalShellField'
import { useAgentApprovalMode } from '@/features/agent-approval/AgentApprovalProvider'
import { ApprovalRulesEditor } from './ApprovalRulesEditor'
import { applyReadingTypographySettings, applyUIFontSize } from './font-variables'
import {
  DEFAULT_READING_FONT_SIZE,
  DEFAULT_UI_FONT_SIZE,
  READING_FONT_SIZE_STEPS,
  UI_FONT_SIZE_STEPS,
} from './font-size-steps'
import { TextSizeControl } from './TextSizeControl'
import { LANAccessSettings } from './LANAccessSettings'
import { UpdatePanel, useUpdateSettings } from './UpdateSettings'
import { MarketSettings } from './MarketSettings'
import { SettingsNavigation } from './SettingsNavigation'
import { SETTINGS_SECTIONS, type SettingsSectionId } from './settings-sections'
import { useSettingsNavigation } from './use-settings-navigation'
import { modelProfilesForEditor, modelEndpointsForEditor, imageAPIProfilesForEditor, imageAPIEndpointsForEditor } from './settings-editor-draft'
import { FieldRow, TextSizeRow, ReadOnly, Text, Num, BoolTri, AgentApprovalModeSelect, ShellEnvironmentSelect, TraceCaptureSelect, TraceExporterSelect, LanguageSelect, ThemeSelect, MotionIntensitySelect, TellerSelect } from './SettingsFields'

export function SettingsView({ visible = true }: { visible?: boolean }) {
  const { t } = useTranslation()
  const approval = useAgentApprovalMode()
  // Every section shares one draft and autosave lane.
  const { layered, draft, setDraft, error, autosaveStatus, autosaveError, saveNow, reload } = useLayeredSettingsDraft({
    target: GLOBAL_SETTINGS_TARGET,
    layer: 'user',
    sourcePrefix: 'settings-view',
  })
  const [availableTellers, setAvailableTellers] = useState<Teller[]>([])
  const { contentRef, activeSection, fieldRequest, selectSection } = useSettingsNavigation({ visible, ready: Boolean(layered) })
  const [revokingApprovalRuleID, setRevokingApprovalRuleID] = useState('')
  const [search, setSearch] = useState('')

  useEffect(() => {
    getInteractiveTellers()
      .then((items) => setAvailableTellers(items))
      .catch((e) => console.warn('[settings] Failed to load the director list', e))
  }, [])

  const effective = layered?.effective ?? {}
  const updatePanel = useUpdateSettings({ autoCheckEnabled: Boolean(layered) && effective.update_check_enabled !== false })
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
    return t('common.defaultValue', { value: String(v) })
  }

  const webAccessPlaceholderFor = (key: keyof WebAccessSettings): string => {
    const value = inherited.web_access?.[key]
    if (value === undefined || value === null || value === '') return t('common.notSet')
    return t('common.defaultValue', { value: String(value) })
  }

  const sections: Record<SettingsSectionId, ReactNode> = {
    'general': (
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
        <div data-onboarding-anchor="settings-onboarding">
          <SettingsFieldRow title={t('settings.onboarding.title')} description={t('settings.onboarding.description')} controlClassName="shrink-0">
            <Button
              type="button"
              size="xs"
              variant="ghost"
              className="shrink-0 text-[var(--nova-text-muted)]"
              onClick={() => {
                selectSection('model')
                window.dispatchEvent(new CustomEvent(ONBOARDING_OPEN_EVENT))
              }}
            >
              {t('settings.onboarding.reopen')}
            </Button>
          </SettingsFieldRow>
        </div>
      </>
    ),
    'writing': (
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
        <Text label={t('settings.agent.writingSkillDefault')} value={draft.writing_skill_default}
              placeholder={placeholderFor('writing_skill_default')}
              onChange={(v) => setField('writing_skill_default', v)} />
      </>
    ),
    'model': (
      <>
        <ModelProfilesEditor fieldRequest={fieldRequest?.id === 'model' ? fieldRequest : undefined}
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
    'image': (
      <>
        <ImageAPIProfilesEditor fieldRequest={fieldRequest?.id === 'image' ? fieldRequest : undefined}
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
    'speech': (
      <SpeechSettingsEditor value={draft.speech ?? effective.speech ?? EMPTY_SPEECH_SETTINGS} onChange={value => setField('speech', value)} visible={visible} />
    ),
    'approvals': (
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
      </>
    ),
    'terminal': (
      <>
        <BoolTri label={t('settings.terminal.enabled')} value={draft.terminal_enabled ?? null}
                 inherited={inherited.terminal_enabled}
                 onChange={(v) => setField('terminal_enabled', v)} />
        <TerminalShellField value={draft.terminal_shell} inherited={inherited.terminal_shell}
                            windows={layered?.runtime?.goos === 'windows'}
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
    'web-access': (
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
    'market': (
      <MarketSettings value={draft.market} defaultURL={layered?.default.market?.registry_url || ''}
        inheritedURL={inherited.market?.registry_url || ''} onChange={(value) => setField('market', value)} />
    ),
    'paths': (
      <>
        <Text label={t('settings.paths.skillsDir')} value={draft.skills_dir} placeholder={placeholderFor('skills_dir')}
              onChange={(v) => setField('skills_dir', v)} />
        <ReadOnly label={t('settings.paths.novaDir')} value={layered?.paths?.denova_dir || layered?.paths?.nova_dir} />
        <ReadOnly label={t('settings.paths.userConfig')} value={layered?.paths?.user_config} />
      </>
    ),
    'versions': (
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
    'access': (
      visible && <LANAccessSettings draft={draft} inherited={inherited}
        onChange={patch => setDraft(current => ({ ...current, ...patch }))} />
    ),
    'updates': (
      <>
        <BoolTri label={t('settings.updates.autoCheck')} value={draft.update_check_enabled ?? null}
                 inherited={inherited.update_check_enabled}
                 onChange={(v) => setField('update_check_enabled', v)} />
        <UpdatePanel {...updatePanel} />
      </>
    ),
    'advanced': (
      <>
        <SettingsBlock title={t('settings.advanced.execution')}>
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
        </SettingsBlock>
        <SettingsBlock title={t('settings.section.labs')}>
          <div className="mb-3 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-2 text-[11px] leading-4 text-[var(--nova-text-muted)]">
            {t('settings.labs.developerModeHint')}
          </div>
          <BoolTri
            label={t('settings.labs.developerMode')}
            value={draft.labs?.developer_mode ?? null}
            inherited={inherited.labs?.developer_mode}
            onChange={(value) => setLabField('developer_mode', value)}
          />
        </SettingsBlock>
        {showDebugSettings && (
          <SettingsBlock title={t('settings.section.debug')}>
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
          </SettingsBlock>
        )}
      </>
    ),
  }

  return (
    <SettingsPageFrame
      visible={visible}
      title={t('settings.title')}
      className="nova-settings-view"
      error={error}
      errorTitle={t('settings.error.save')}
      navigation={layered ? (
        <SettingsNavigation activeId={activeSection} search={search} onSearchChange={setSearch} onSelect={(id, fieldKey) => { selectSection(id, fieldKey); setSearch('') }}
          showDebug={showDebugSettings} goos={layered.runtime?.goos} />
      ) : undefined}
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
        <div ref={contentRef} data-nova-settings-content="true" className="h-full min-h-0 overflow-y-auto overscroll-contain px-4 py-5 sm:px-6 [container-type:size]">
          <div className="mx-auto flex w-full min-w-0 max-w-3xl flex-col gap-8">
            {/* Keep the final anchor able to align with the top of the viewport. */}
            {SETTINGS_SECTIONS.map((section) => (
              <section key={section.id} data-settings-section={section.id} data-onboarding-anchor={section.id === 'model' ? 'settings-model' : undefined} className="flex min-w-0 flex-col gap-3 last:min-h-[calc(100cqh-2.5rem)]">
                <h2 className="text-lg font-semibold">{t(section.titleKey)}</h2>
                <div className="nova-settings-section-card flex flex-col gap-3 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] p-3 sm:p-4">
                  {sections[section.id]}
                </div>
              </section>
            ))}
          </div>
        </div>
      )}
    </SettingsPageFrame>
  )
}

function SettingsBlock({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div className="flex flex-col gap-2 border-b border-[var(--nova-border)] pb-4 last:border-b-0 last:pb-0">
      <h3 className="px-2 text-sm font-medium">{title}</h3>
      {children}
    </div>
  )
}
