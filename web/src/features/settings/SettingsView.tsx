import { cloneElement, isValidElement, useEffect, useId, useMemo, useRef, useState, useCallback } from 'react'
import type { ReactNode } from 'react'
import { ChevronDown, ChevronUp, Download, ExternalLink, Loader2, Plus, RefreshCw, Settings as SettingsIcon, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { ImageAPIProfileSettings, ModelProfileSettings, Settings, UpdateApplyResult, UpdateCheckResult, UpdateInstallProgress, UpdateInstallResult } from './types'
import { applyUpdate, checkForUpdate, installUpdateStream } from './api'
import { FONT_OPTIONS, fontLabelKeyFor } from './font-options'
import { useLayeredSettingsDraft } from './use-layered-settings-draft'
import { getInteractiveTellers } from '@/features/interactive/api'
import type { Teller } from '@/features/interactive/types'
import { InlineErrorNotice } from '@/components/common/inline-error-notice'
import { AutosaveStatusIndicator } from '@/components/forms/autosave-status'
import { SettingsFieldRow } from '@/components/forms/settings-field-row'
import { AdaptiveSurface } from '@/components/layout/adaptive-surface'
import { FeaturePageShell } from '@/components/layout/feature-page-shell'
import { MobilePaneTrigger } from '@/components/layout/mobile-pane-trigger'
import { SectionedNavigation } from '@/components/navigation/sectioned-navigation'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Separator } from '@/components/ui/separator'
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { LOCALE_OPTIONS } from '@/i18n'
import { APP_VERSION } from '@/app-version'
import { markAutoUpdateChecked, notifyUpdateCheckResult, shouldRunAutoUpdateCheck } from './update-check-cache'
import { scheduleFrontendReloadAfterUpdate } from './update-reload'
import { DEFAULT_MODEL_PROFILE_ID, modelProfileID, modelProfileLabel, modelProfilesWithDefault } from './model-profiles'
import { DEFAULT_IMAGE_API_BASE_URL, DEFAULT_IMAGE_API_MODEL, DEFAULT_IMAGE_API_PROFILE_ID, DEFAULT_IMAGE_API_PROVIDER, imageAPIProfileID, imageAPIProfileLabel, imageAPIProfilesWithDefault } from './image-profiles'
import { ONBOARDING_OPEN_EVENT, SETTINGS_SECTION_EVENT, type SettingsSectionRequest } from '@/features/onboarding/events'

type SettingsSectionId = 'model' | 'image' | 'paths' | 'access' | 'appearance' | 'updates' | 'agent' | 'debug' | 'ide-editor' | 'ide-output' | 'versions' | 'interactive'

const SETTINGS_SECTION_IDS: SettingsSectionId[] = ['model', 'image', 'paths', 'access', 'appearance', 'updates', 'agent', 'debug', 'ide-editor', 'ide-output', 'versions', 'interactive']

type SettingsSection = {
  id: SettingsSectionId
  group: string
  title: string
  children: ReactNode
}

const fieldCls = 'nova-field min-h-7 flex-1 rounded-[var(--nova-radius)] border px-2.5 py-1.5 outline-none placeholder:text-[var(--nova-text-faint)] focus:border-[var(--nova-field-focus-border)] focus:bg-[var(--nova-surface-3)]'
const DEFAULT_CONTEXT_WINDOW_TOKENS = 400000
const MIN_CONTEXT_WINDOW_TOKENS = 1024
const MAX_CONTEXT_WINDOW_TOKENS = 2000000
const CONTEXT_WINDOW_PRESETS = [200000, DEFAULT_CONTEXT_WINDOW_TOKENS, 1000000]
const CONTEXT_WINDOW_INHERIT_VALUE = 'inherit'
const IMAGE_API_INHERIT_VALUE = '__inherit__'
const IMAGE_API_PROVIDER_DEFAULT_VALUE = '__provider_default__'
const IMAGE_API_QUALITY_OPTIONS = ['auto', 'high', 'medium', 'low', 'standard', 'hd']
const IMAGE_API_FORMAT_OPTIONS = ['png', 'jpeg']
const TRACE_CAPTURE_OPTIONS = [
  { value: 'summary', labelKey: 'settings.debug.traceCaptureSummary' },
  { value: 'debug', labelKey: 'settings.debug.traceCaptureDebug' },
  { value: 'off', labelKey: 'settings.debug.traceCaptureOff' },
] as const
const TRACE_EXPORTER_OPTIONS = [
  { value: 'local', labelKey: 'settings.debug.traceExporterLocal' },
] as const
export function SettingsView({ onClose }: { onClose?: () => void }) {
  const { t } = useTranslation()
  const { layered, draft, setDraft, error, autosaveStatus, autosaveError, saveNow } = useLayeredSettingsDraft({
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
  const [expandedSections, setExpandedSections] = useState<Record<SettingsSectionId, boolean>>({
    model: true,
    image: true,
    paths: true,
    access: true,
    appearance: true,
    updates: true,
    agent: true,
    debug: true,
    'ide-editor': true,
    'ide-output': true,
    versions: true,
    interactive: true,
  })
  const contentRef = useRef<HTMLDivElement | null>(null)
  const sectionRefs = useRef<Record<string, HTMLElement | null>>({})

  useEffect(() => {
    getInteractiveTellers()
      .then((items) => setAvailableTellers(items))
      .catch((e) => console.warn('[settings] 获取导演列表失败', e))
  }, [])

  const effective = layered?.effective ?? {}
  const showDebugSettings = layered?.runtime?.dev_mode === true

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

  const setImageAPIProfiles = (profiles: ImageAPIProfileSettings[]) => {
    setDraft((d) => ({
      ...d,
      image_api_key: '',
      image_api_base_url: '',
      image_api_model: '',
      image_api_profiles: profiles,
    }))
  }

  const placeholderFor = (k: keyof Settings): string => {
    const v = effective[k]
    if (v === undefined || v === null || v === '') return t('common.notSet')
    return t('common.inherit', { value: String(v) })
  }

  const sections: SettingsSection[] = [
    {
      id: 'appearance',
      group: t('settings.group.common'),
      title: t('settings.section.appearance'),
      children: (
        <>
          <LanguageSelect label={t('settings.appearance.language')} value={draft.language}
                          effective={effective.language}
                          onChange={(v) => setField('language', v)} />
          <ThemeSelect label={t('settings.appearance.theme')} value={draft.theme}
                       effective={effective.theme}
                       onChange={(v) => setField('theme', v)} />
          <MotionIntensitySelect label={t('settings.appearance.motionIntensity')} value={draft.motion_intensity}
                                 effective={effective.motion_intensity}
                                 onChange={(v) => setField('motion_intensity', v)} />
          <FontSelect label={t('settings.appearance.uiFont')} value={draft.ui_font_family}
                      effective={effective.ui_font_family}
                      onChange={(v) => setField('ui_font_family', v)} />
          <Num label={t('settings.appearance.uiFontSize')} value={draft.ui_font_size ?? null}
               placeholder={placeholderFor('ui_font_size')}
               min={11}
               max={16}
               onChange={(v) => setField('ui_font_size', v)} />
          <FontSelect label={t('settings.appearance.readingFont')} value={draft.reading_font_family}
                      effective={effective.reading_font_family}
                      onChange={(v) => setField('reading_font_family', v)} />
          <Num label={t('settings.appearance.readingFontSize')} value={draft.reading_font_size ?? null}
               placeholder={placeholderFor('reading_font_size')}
               min={14}
               max={28}
               onChange={(v) => setField('reading_font_size', v)} />
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
                   effective={effective.update_check_enabled}
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
            profiles={modelProfilesForEditor(draft, effective)}
            effectiveProfiles={modelProfilesWithDefault(effective)}
            onChange={setModelProfiles}
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
            profiles={imageAPIProfilesForEditor(draft, effective)}
            effectiveProfiles={imageAPIProfilesWithDefault(effective)}
            defaultProfileID={draft.default_image_api_profile_id ?? ''}
            effectiveDefaultProfileID={effective.default_image_api_profile_id || DEFAULT_IMAGE_API_PROFILE_ID}
            onDefaultProfileChange={(v) => setField('default_image_api_profile_id', v)}
            onChange={setImageAPIProfiles}
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
                   effective={effective.allow_lan_access}
                   onChange={(v) => setField('allow_lan_access', v)} />
          <Text label={t('settings.access.username')} value={draft.remote_access_username}
                placeholder={placeholderFor('remote_access_username')}
                onChange={(v) => setField('remote_access_username', v)} />
          <Text label={t('settings.access.password')} value={draft.remote_access_password}
                placeholder={(draft.remote_access_password_set || effective.remote_access_password_set)
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
          <BoolTri label={t('settings.agent.planModeDefault')} value={draft.plan_mode_default ?? null}
                   effective={effective.plan_mode_default}
                   onChange={(v) => setField('plan_mode_default', v)} />
          <Text label={t('settings.agent.writingSkillDefault')} value={draft.writing_skill_default}
                placeholder={placeholderFor('writing_skill_default')}
                onChange={(v) => setField('writing_skill_default', v)} />
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
                   effective={effective.llm_input_log_enabled}
                   onChange={(v) => setField('llm_input_log_enabled', v)} />
          <TraceCaptureSelect label={t('settings.debug.traceCaptureLevel')} value={draft.trace_capture_level}
                              effective={effective.trace_capture_level}
                              onChange={(v) => setField('trace_capture_level', v)} />
          <TraceExporterSelect label={t('settings.debug.traceExporter')} value={draft.trace_exporter}
                               effective={effective.trace_exporter}
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
                   effective={effective.auto_save_enabled}
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
            effective={effective.ide_story_teller_id}
            tellers={availableTellers}
            onChange={(v) => setField('ide_story_teller_id', v)}
          />
        </>
      ),
    },
    {
      id: 'ide-output',
      group: t('settings.group.ide'),
      title: t('settings.section.liveOutput'),
      children: (
        <>
          <BoolTri label={t('settings.ide.hideNovelChapterBodyInLiveOutput')} value={draft.hide_novel_chapter_body_in_live_output ?? null}
                   effective={effective.hide_novel_chapter_body_in_live_output}
                   onChange={(v) => setField('hide_novel_chapter_body_in_live_output', v)} />
          <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-2 text-xs leading-5 text-[var(--nova-text-faint)]">
            {t('settings.ide.hideNovelChapterBodyInLiveOutputHelp')}
          </div>
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
                   effective={effective.version_timed_enabled}
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
          <Num label={t('settings.interactive.lineHeight')} value={draft.interactive_stage_line_height ?? null}
               placeholder={placeholderFor('interactive_stage_line_height')}
               step={0.05}
               onChange={(v) => setField('interactive_stage_line_height', v)} />
        </>
      ),
    },
  ]

  const jumpToSection = useCallback((id: SettingsSectionId) => {
    setActiveSection(id)
    setExpandedSections((prev) => ({ ...prev, [id]: true }))
    requestAnimationFrame(() => {
      sectionRefs.current[id]?.scrollIntoView({ block: 'start', behavior: 'smooth' })
    })
  }, [])

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
    const container = contentRef.current
    if (!container) return
    const top = container.getBoundingClientRect().top
    const current = sections.reduce<SettingsSectionId>((acc, section) => {
      const node = sectionRefs.current[section.id]
      if (!node) return acc
      return node.getBoundingClientRect().top <= top + 72 ? section.id : acc
    }, sections[0]?.id ?? 'model')
    if (current !== activeSection) setActiveSection(current)
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
      className="h-full min-h-0 overflow-y-auto bg-[var(--nova-surface-2)] px-2 py-4 sm:px-3"
      itemClassName="font-normal"
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
        desktopGridClassName="grid-cols-[14rem_minmax(0,1fr)]"
      >
        {({ openLeft }) => (
          <div ref={contentRef} data-nova-settings-content="true" onScroll={onContentScroll} className="h-full min-h-0 overflow-y-auto overscroll-contain px-4 py-5 sm:px-6">
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
    </FeaturePageShell>
  )
}

export function modelProfilesForEditor(draft: Settings, effective: Settings): ModelProfileSettings[] {
  const localProfiles = draft.model_profiles ?? []
  const hasLocalDefault = localProfiles.some((profile) => modelProfileID(profile) === DEFAULT_MODEL_PROFILE_ID)
  const hasLegacyDefault = Boolean(draft.openai_api_key || draft.openai_base_url || draft.openai_model || draft.openai_context_window_tokens)
  if (hasLocalDefault || hasLegacyDefault) {
    return preserveDraftOnlyModelProfiles(modelProfilesWithDefault(draft), localProfiles)
  }
  const inherited = modelProfilesWithDefault(effective)
  const localIDs = new Set(localProfiles.map(modelProfileID).filter(Boolean))
  return [
    ...inherited.filter((profile) => !localIDs.has(modelProfileID(profile))).map(stripInheritedModelSecret),
    ...localProfiles,
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

function stripInheritedModelSecret(profile: ModelProfileSettings): ModelProfileSettings {
  return { ...profile, openai_api_key: '' }
}

function imageAPIProfilesForEditor(draft: Settings, effective: Settings): ImageAPIProfileSettings[] {
  const localProfiles = draft.image_api_profiles ?? []
  const hasLocalDefault = localProfiles.some((profile) => imageAPIProfileID(profile) === DEFAULT_IMAGE_API_PROFILE_ID)
  const hasLegacyDefault = Boolean(draft.image_api_key || draft.image_api_base_url || draft.image_api_model)
  if (hasLocalDefault || hasLegacyDefault) {
    return imageAPIProfilesWithDefault(draft)
  }
  const inherited = imageAPIProfilesWithDefault(effective)
  const localIDs = new Set(localProfiles.map(imageAPIProfileID).filter(Boolean))
  return [
    ...inherited.filter((profile) => !localIDs.has(imageAPIProfileID(profile))).map(stripInheritedImageAPISecret),
    ...localProfiles,
  ]
}

function stripInheritedImageAPISecret(profile: ImageAPIProfileSettings): ImageAPIProfileSettings {
  return { ...profile, openai_api_key: '' }
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

function BoolTri({ label, value, effective, onChange }: {
  label: string; value: boolean | null; effective?: boolean | null
  onChange: (v: boolean | null) => void
}) {
  const { t } = useTranslation()
  const eff = effective === null || effective === undefined ? t('common.notSet') : String(effective)
  return (
    <FieldRow label={label}>
      <select
        value={value === null ? '' : String(value)}
        onChange={(e) => {
          const v = e.target.value
          onChange(v === '' ? null : v === 'true')
        }}
        className={fieldCls}
      >
        <option value="">{t('common.inherit', { value: eff })}</option>
        <option value="true">{t('settings.bool.true')}</option>
        <option value="false">{t('settings.bool.false')}</option>
      </select>
    </FieldRow>
  )
}

function TraceCaptureSelect({ label, value, effective, onChange }: {
  label: string
  value?: string
  effective?: string
  onChange: (v: string) => void
}) {
  const { t } = useTranslation()
  const effectiveValue = effective || 'summary'
  const effectiveLabel = t(TRACE_CAPTURE_OPTIONS.find((option) => option.value === effectiveValue)?.labelKey || 'settings.debug.traceCaptureSummary')
  return (
    <FieldRow label={label}>
      <select
        value={value ?? ''}
        onChange={(e) => onChange(e.target.value)}
        className={fieldCls}
      >
        <option value="">{t('common.inherit', { value: effectiveLabel })}</option>
        {TRACE_CAPTURE_OPTIONS.map((option) => (
          <option key={option.value} value={option.value}>{t(option.labelKey)}</option>
        ))}
      </select>
    </FieldRow>
  )
}

function TraceExporterSelect({ label, value, effective, onChange }: {
  label: string
  value?: string
  effective?: string
  onChange: (v: string) => void
}) {
  const { t } = useTranslation()
  const effectiveValue = TRACE_EXPORTER_OPTIONS.some((option) => option.value === effective) ? effective || 'local' : 'local'
  const selectedValue = TRACE_EXPORTER_OPTIONS.some((option) => option.value === value) ? value || '' : ''
  const effectiveLabel = t(TRACE_EXPORTER_OPTIONS.find((option) => option.value === effectiveValue)?.labelKey || 'settings.debug.traceExporterLocal')
  return (
    <FieldRow label={label}>
      <select
        value={selectedValue}
        onChange={(e) => onChange(e.target.value)}
        className={fieldCls}
      >
        <option value="">{t('common.inherit', { value: effectiveLabel })}</option>
        {TRACE_EXPORTER_OPTIONS.map((option) => (
          <option key={option.value} value={option.value}>{t(option.labelKey)}</option>
        ))}
      </select>
    </FieldRow>
  )
}

function FontSelect({ label, value, effective, onChange }: {
  label: string
  value?: string
  effective?: string
  onChange: (v: string) => void
}) {
  const { t } = useTranslation()
  const effectiveLabelKey = fontLabelKeyFor(effective)
  const effectiveLabel = effectiveLabelKey ? t(effectiveLabelKey) : (effective || t('common.notSet'))
  return (
    <FieldRow label={label}>
      <select
        value={value ?? ''}
        onChange={(e) => onChange(e.target.value)}
        className={fieldCls}
      >
        <option value="">{t('common.inherit', { value: effectiveLabel })}</option>
        {FONT_OPTIONS.map((font) => (
          <option key={font.value} value={font.value}>{t(font.labelKey)}</option>
        ))}
      </select>
    </FieldRow>
  )
}

function LanguageSelect({ label, value, effective, onChange }: {
  label: string
  value?: string
  effective?: string
  onChange: (v: string) => void
}) {
  const { t } = useTranslation()
  const effectiveLabel = t(LOCALE_OPTIONS.find((option) => option.value === (effective || 'auto'))?.labelKey || 'locale.auto')
  return (
    <FieldRow label={label}>
      <select
        value={value ?? ''}
        onChange={(e) => onChange(e.target.value)}
        className={fieldCls}
      >
        <option value="">{t('common.inherit', { value: effectiveLabel })}</option>
        {LOCALE_OPTIONS.map((option) => (
          <option key={option.value} value={option.value}>{t(option.labelKey)}</option>
        ))}
      </select>
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

function ThemeSelect({ label, value, effective, onChange }: {
  label: string
  value?: string
  effective?: string
  onChange: (v: string) => void
}) {
  const { t } = useTranslation()
  const effectiveValue = effective || 'dark'
  const effectiveLabel = t(THEME_OPTIONS.find((option) => option.value === effectiveValue)?.labelKey || 'settings.theme.dark')
  return (
    <FieldRow label={label}>
      <select
        value={value ?? ''}
        onChange={(e) => onChange(e.target.value)}
        className={fieldCls}
      >
        <option value="">{t('common.inherit', { value: effectiveLabel })}</option>
        {THEME_OPTIONS.map((option) => (
          <option key={option.value} value={option.value}>{t(option.labelKey)}</option>
        ))}
      </select>
    </FieldRow>
  )
}

function MotionIntensitySelect({ label, value, effective, onChange }: {
  label: string
  value?: string
  effective?: string
  onChange: (v: string) => void
}) {
  const { t } = useTranslation()
  const effectiveValue = effective || 'system'
  const effectiveLabel = t(MOTION_INTENSITY_OPTIONS.find((option) => option.value === effectiveValue)?.labelKey || 'settings.motion.system')
  return (
    <FieldRow label={label}>
      <select
        value={value ?? ''}
        onChange={(e) => onChange(e.target.value)}
        className={fieldCls}
      >
        <option value="">{t('common.inherit', { value: effectiveLabel })}</option>
        {MOTION_INTENSITY_OPTIONS.map((option) => (
          <option key={option.value} value={option.value}>{t(option.labelKey)}</option>
        ))}
      </select>
    </FieldRow>
  )
}

function TellerSelect({ label, value, effective, tellers, onChange }: {
  label: string
  value?: string
  effective?: string
  tellers: Teller[]
  onChange: (v: string) => void
}) {
  const { t } = useTranslation()
  const effectiveName = tellers.find((teller) => teller.id === effective)?.name || effective || 'classic'
  return (
    <FieldRow label={label}>
      <select
        value={value ?? ''}
        onChange={(e) => onChange(e.target.value)}
        className={fieldCls}
      >
        <option value="">{t('common.inherit', { value: effectiveName })}</option>
        {tellers.map((teller) => (
          <option key={teller.id} value={teller.id}>{teller.name}</option>
        ))}
      </select>
    </FieldRow>
  )
}

function ModelProfilesEditor({ profiles, effectiveProfiles, onChange }: {
  profiles: ModelProfileSettings[]
  effectiveProfiles: ModelProfileSettings[]
  onChange: (profiles: ModelProfileSettings[]) => void
}) {
  const { t } = useTranslation()
  const profileKeysRef = useRef<string[]>([])
  const profileKeys = useMemo(() => {
    if (profileKeysRef.current.length > profiles.length) {
      profileKeysRef.current = profileKeysRef.current.slice(0, profiles.length)
    }
    while (profileKeysRef.current.length < profiles.length) {
      profileKeysRef.current.push(`profile-${Date.now()}-${profileKeysRef.current.length}`)
    }
    return profileKeysRef.current
  }, [profiles.length])
  const addProfile = () => {
    onChange([...profiles, { context_window_tokens: DEFAULT_CONTEXT_WINDOW_TOKENS }])
  }
  const updateProfile = (index: number, patch: Partial<ModelProfileSettings>) => {
    onChange(profiles.map((profile, i) => (i === index ? { ...profile, ...patch } : profile)))
  }
  const updateProfileModel = (index: number, openaiModel: string) => {
    const profile = profiles[index]
    const previousID = modelProfileID(profile)
    const previousModel = profile?.openai_model?.trim() ?? ''
    const shouldSyncID = !previousID || previousID === previousModel
    updateProfile(index, {
      id: shouldSyncID ? openaiModel : profile?.id,
      openai_model: openaiModel,
    })
  }
  const removeProfile = (index: number) => {
    onChange(profiles.filter((_, i) => i !== index))
  }

  return (
    <div className="nova-settings-row rounded-md px-2 py-1.5">
      <div className="mb-1.5 text-[var(--nova-text-muted)]">{t('settings.model.modelProfiles')}</div>
      <div className="flex flex-col gap-2">
        {profiles.length === 0 && (
          <div className="rounded-[var(--nova-radius)] border border-dashed border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2.5 py-2 text-[var(--nova-text-faint)]">
            {t('settings.model.profileEmpty', { count: effectiveProfiles.length || 1 })}
          </div>
        )}
        {profiles.map((profile, index) => {
          const isDefaultProfile = modelProfileID(profile) === DEFAULT_MODEL_PROFILE_ID
          return (
          <div key={profileKeys[index]} className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)]">
            <div className="flex items-center gap-2 px-2.5 py-2">
              <Badge variant="outline" className="shrink-0">
                {isDefaultProfile ? t('settings.model.defaultProfileName') : t('settings.model.profileName', { index: index + 1 })}
              </Badge>
              <div className="min-w-0 flex-1">
                <div className="truncate text-xs font-medium text-[var(--nova-text)]">
                  {modelProfileLabel(profile) || t('settings.model.profileUntitled')}
                </div>
                <div className="truncate text-[11px] text-[var(--nova-text-faint)]">
                  {profile.openai_model?.trim() || t('settings.model.profileModelMissing')}
                </div>
              </div>
              <Button
                type="button"
                variant="outline"
                size="icon-sm"
                onClick={() => removeProfile(index)}
                aria-label={t('settings.model.deleteProfile')}
                title={t('settings.model.deleteProfile')}
              >
                <Trash2 data-icon="inline-start" />
              </Button>
            </div>
            <Separator />
            <div className="grid gap-2 p-2.5 md:grid-cols-12">
              <ModelProfileInput label={t('common.baseUrl')} className="md:col-span-5">
                <Input
                  value={profile.openai_base_url ?? ''}
                  placeholder={t('common.baseUrl')}
                  onChange={(e) => updateProfile(index, { openai_base_url: e.target.value })}
                />
              </ModelProfileInput>
              <ModelProfileInput label={t('settings.model.profileModelLabel')} className="md:col-span-4">
                <Input
                  value={profile.openai_model ?? ''}
                  placeholder={t('settings.model.profileModelPlaceholder')}
                  onChange={(e) => updateProfileModel(index, e.target.value)}
                />
              </ModelProfileInput>
              <ModelProfileInput label={t('settings.model.profileAliasLabel')} className="md:col-span-3">
                <Input
                  value={profile.name ?? ''}
                  placeholder={t('settings.model.profileAliasPlaceholder')}
                  onChange={(e) => updateProfile(index, { name: e.target.value })}
                />
              </ModelProfileInput>
              <ModelProfileInput label={t('settings.model.profileKeyLabel')} className="md:col-span-5">
                <Input
                  type="password"
                  value={profile.openai_api_key ?? ''}
                  placeholder={t('settings.model.profileKeyInheritPlaceholder')}
                  onChange={(e) => updateProfile(index, { openai_api_key: e.target.value })}
                />
              </ModelProfileInput>
              <ModelProfileInput label={t('settings.model.profileTemperatureLabel')} className="md:col-span-2">
                <Input
                  type="number"
                  step={0.01}
                  min={0}
                  max={1}
                  value={profile.temperature ?? ''}
                  placeholder="0-1"
                  onChange={(e) => updateProfile(index, { temperature: e.target.value === '' ? null : Number(e.target.value) })}
                  className="max-w-24"
                />
              </ModelProfileInput>
              <ModelProfileInput label={t('settings.model.contextWindow')} className="md:col-span-5">
                <ContextWindowInput
                  value={profile.context_window_tokens ?? DEFAULT_CONTEXT_WINDOW_TOKENS}
                  onChange={(value) => updateProfile(index, { context_window_tokens: value })}
                />
              </ModelProfileInput>
            </div>
          </div>
          )
        })}
        <Button
          type="button"
          onClick={addProfile}
          variant="outline"
          size="sm"
        >
          <Plus data-icon="inline-start" />
          {t('settings.model.addProfile')}
        </Button>
      </div>
    </div>
  )
}

function ImageAPIProfilesEditor({ profiles, effectiveProfiles, defaultProfileID, effectiveDefaultProfileID, onDefaultProfileChange, onChange }: {
  profiles: ImageAPIProfileSettings[]
  effectiveProfiles: ImageAPIProfileSettings[]
  defaultProfileID: string
  effectiveDefaultProfileID: string
  onDefaultProfileChange: (profileID: string) => void
  onChange: (profiles: ImageAPIProfileSettings[]) => void
}) {
  const { t } = useTranslation()
  const profileKeysRef = useRef<string[]>([])
  const profileKeys = useMemo(() => {
    if (profileKeysRef.current.length > profiles.length) {
      profileKeysRef.current = profileKeysRef.current.slice(0, profiles.length)
    }
    while (profileKeysRef.current.length < profiles.length) {
      profileKeysRef.current.push(`image-profile-${Date.now()}-${profileKeysRef.current.length}`)
    }
    return profileKeysRef.current
  }, [profiles.length])
  const profileOptions = imageProfileOptions(profiles, effectiveProfiles)
  const effectiveDefaultLabel = profileOptions.find((profile) => profile.id === effectiveDefaultProfileID)?.label || effectiveDefaultProfileID || DEFAULT_IMAGE_API_PROFILE_ID
  const addProfile = () => {
    onChange([...profiles, {
      provider: DEFAULT_IMAGE_API_PROVIDER,
      openai_base_url: DEFAULT_IMAGE_API_BASE_URL,
      openai_model: DEFAULT_IMAGE_API_MODEL,
    }])
  }
  const updateProfile = (index: number, patch: Partial<ImageAPIProfileSettings>) => {
    onChange(profiles.map((profile, i) => (i === index ? { ...profile, ...patch } : profile)))
  }
  const updateProfileModel = (index: number, openaiModel: string) => {
    const profile = profiles[index]
    const previousID = imageAPIProfileID(profile)
    const previousModel = profile?.openai_model?.trim() ?? ''
    const shouldSyncID = !previousID || previousID === previousModel
    updateProfile(index, {
      id: shouldSyncID ? openaiModel : profile?.id,
      openai_model: openaiModel,
    })
  }
  const removeProfile = (index: number) => {
    const removedID = imageAPIProfileID(profiles[index])
    onChange(profiles.filter((_, i) => i !== index))
    if (removedID && defaultProfileID === removedID) onDefaultProfileChange('')
  }

  return (
    <div className="nova-settings-row rounded-md px-2 py-1.5">
      <div className="mb-1.5 text-[var(--nova-text-muted)]">{t('settings.imageApi.profiles')}</div>
      <div className="flex flex-col gap-2">
        <ModelProfileInput label={t('settings.imageApi.defaultProfile')}>
          <Select
            value={defaultProfileID || IMAGE_API_INHERIT_VALUE}
            onValueChange={(value) => onDefaultProfileChange(value === IMAGE_API_INHERIT_VALUE ? '' : value)}
          >
            <SelectTrigger size="sm" className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent className="nova-panel border text-[var(--nova-text)]">
              <SelectGroup>
                <SelectItem value={IMAGE_API_INHERIT_VALUE}>{t('common.inherit', { value: effectiveDefaultLabel })}</SelectItem>
                {profileOptions.map((profile) => (
                  <SelectItem key={profile.id} value={profile.id}>{profile.label}</SelectItem>
                ))}
              </SelectGroup>
            </SelectContent>
          </Select>
        </ModelProfileInput>
        {profiles.length === 0 && (
          <div className="rounded-[var(--nova-radius)] border border-dashed border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2.5 py-2 text-[var(--nova-text-faint)]">
            {t('settings.imageApi.profileEmpty', { count: effectiveProfiles.length || 1 })}
          </div>
        )}
        {profiles.map((profile, index) => (
          <div key={profileKeys[index]} className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)]">
            <div className="flex items-center gap-2 px-2.5 py-2">
              <Badge variant="outline" className="shrink-0">
                {t('settings.imageApi.profileName', { index: index + 1 })}
              </Badge>
              <div className="min-w-0 flex-1">
                <div className="truncate text-xs font-medium text-[var(--nova-text)]">
                  {imageAPIProfileLabel(profile) || t('settings.imageApi.profileUntitled')}
                </div>
                <div className="truncate text-[11px] text-[var(--nova-text-faint)]">
                  {profile.openai_model?.trim() || t('settings.imageApi.profileModelMissing')}
                </div>
              </div>
              <Button
                type="button"
                variant="outline"
                size="icon-sm"
                onClick={() => removeProfile(index)}
                aria-label={t('settings.imageApi.deleteProfile')}
                title={t('settings.imageApi.deleteProfile')}
              >
                <Trash2 data-icon="inline-start" />
              </Button>
            </div>
            <Separator />
            <div className="grid gap-2 p-2.5 md:grid-cols-12">
              <ModelProfileInput label={t('settings.imageApi.provider')} className="md:col-span-3">
                <Select
                  value={profile.provider || DEFAULT_IMAGE_API_PROVIDER}
                  onValueChange={(value) => updateProfile(index, { provider: value })}
                >
                  <SelectTrigger size="sm" className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent className="nova-panel border text-[var(--nova-text)]">
                    <SelectGroup>
                      <SelectItem value={DEFAULT_IMAGE_API_PROVIDER}>OpenAI</SelectItem>
                    </SelectGroup>
                  </SelectContent>
                </Select>
              </ModelProfileInput>
              <ModelProfileInput label={t('common.baseUrl')} className="md:col-span-5">
                <Input
                  value={profile.openai_base_url ?? ''}
                  placeholder={DEFAULT_IMAGE_API_BASE_URL}
                  onChange={(e) => updateProfile(index, { openai_base_url: e.target.value })}
                />
              </ModelProfileInput>
              <ModelProfileInput label={t('settings.imageApi.profileModelLabel')} className="md:col-span-4">
                <Input
                  value={profile.openai_model ?? ''}
                  placeholder={DEFAULT_IMAGE_API_MODEL}
                  onChange={(e) => updateProfileModel(index, e.target.value)}
                />
              </ModelProfileInput>
              <ModelProfileInput label={t('settings.imageApi.profileAliasLabel')} className="md:col-span-3">
                <Input
                  value={profile.name ?? ''}
                  placeholder={t('settings.imageApi.profileAliasPlaceholder')}
                  onChange={(e) => updateProfile(index, { name: e.target.value })}
                />
              </ModelProfileInput>
              <ModelProfileInput label={t('settings.imageApi.profileKeyLabel')} className="md:col-span-5">
                <Input
                  type="password"
                  value={profile.openai_api_key ?? ''}
                  placeholder={t('settings.imageApi.profileKeyInheritPlaceholder')}
                  onChange={(e) => updateProfile(index, { openai_api_key: e.target.value })}
                />
              </ModelProfileInput>
              <ImageOptionSelect
                label={t('settings.imageApi.defaultQuality')}
                value={profile.default_quality ?? ''}
                options={IMAGE_API_QUALITY_OPTIONS}
                placeholder={t('settings.imageApi.providerDefault')}
                className="md:col-span-3"
                onChange={(value) => updateProfile(index, { default_quality: value })}
              />
              <ImageOptionSelect
                label={t('settings.imageApi.defaultOutputFormat')}
                value={profile.default_output_format ?? ''}
                options={IMAGE_API_FORMAT_OPTIONS}
                placeholder={t('settings.imageApi.providerDefault')}
                className="md:col-span-3"
                onChange={(value) => updateProfile(index, { default_output_format: value })}
              />
            </div>
          </div>
        ))}
        <Button
          type="button"
          onClick={addProfile}
          variant="outline"
          size="sm"
        >
          <Plus data-icon="inline-start" />
          {t('settings.imageApi.addProfile')}
        </Button>
      </div>
    </div>
  )
}

function ImageOptionSelect({ label, value, options, placeholder, className, onChange }: {
  label: string
  value: string
  options: string[]
  placeholder: string
  className?: string
  onChange: (value: string) => void
}) {
  return (
    <ModelProfileInput label={label} className={className}>
      <Select value={value || IMAGE_API_PROVIDER_DEFAULT_VALUE} onValueChange={(next) => onChange(next === IMAGE_API_PROVIDER_DEFAULT_VALUE ? '' : next)}>
        <SelectTrigger size="sm" className="w-full">
          <SelectValue />
        </SelectTrigger>
        <SelectContent className="nova-panel border text-[var(--nova-text)]">
          <SelectGroup>
            <SelectItem value={IMAGE_API_PROVIDER_DEFAULT_VALUE}>{placeholder}</SelectItem>
            {options.map((option) => (
              <SelectItem key={option} value={option}>{option}</SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
    </ModelProfileInput>
  )
}

function imageProfileOptions(localProfiles: ImageAPIProfileSettings[], effectiveProfiles: ImageAPIProfileSettings[]) {
  const options: Array<{ id: string; label: string }> = []
  const seen = new Set<string>()
  const add = (profile?: ImageAPIProfileSettings) => {
    const id = imageAPIProfileID(profile)
    if (!id || seen.has(id)) return
    seen.add(id)
    options.push({ id, label: imageAPIProfileLabel(profile) || id })
  }
  effectiveProfiles.forEach(add)
  localProfiles.forEach(add)
  return options
}

function ModelProfileInput({ label, className, children }: { label: string; className?: string; children: ReactNode }) {
  return (
    <label className={`flex min-w-0 flex-col gap-1 ${className ?? ''}`}>
      <span className="text-[11px] leading-none text-[var(--nova-text-faint)]">{label}</span>
      {children}
    </label>
  )
}

function ContextWindowInput({ value, effective, allowInherit = false, onChange }: {
  value: number | null
  effective?: number | null
  allowInherit?: boolean
  onChange: (value: number | null) => void
}) {
  const { t } = useTranslation()
  const [customDraft, setCustomDraft] = useState<string | null>(null)
  const selectedValue = value ?? DEFAULT_CONTEXT_WINDOW_TOKENS
  const customEditing = customDraft !== null
  const preset = value === null && allowInherit && !customEditing
    ? CONTEXT_WINDOW_INHERIT_VALUE
    : (!customEditing && CONTEXT_WINDOW_PRESETS.includes(selectedValue) ? String(selectedValue) : 'custom')
  const custom = preset === 'custom'
  const inheritedValue = effective ?? DEFAULT_CONTEXT_WINDOW_TOKENS
  const customValue = customDraft ?? (value === null ? '' : String(value))
  return (
    <div className="flex min-w-0 flex-1 flex-col gap-2 sm:flex-row">
      <Select
        value={preset}
        onValueChange={(nextValue) => {
          if (nextValue === CONTEXT_WINDOW_INHERIT_VALUE) {
            setCustomDraft(null)
            onChange(null)
            return
          }
          if (nextValue === 'custom') {
            setCustomDraft(value === null ? '' : String(value))
            return
          }
          setCustomDraft(null)
          onChange(Number(nextValue))
        }}
      >
        <SelectTrigger
          size="sm"
          className="min-w-0 flex-1"
          aria-label={t('settings.model.contextWindow')}
          title={t('settings.model.contextWindow')}
        >
          <SelectValue />
        </SelectTrigger>
        <SelectContent className="nova-panel border text-[var(--nova-text)]">
          <SelectGroup>
            {allowInherit && (
              <SelectItem value={CONTEXT_WINDOW_INHERIT_VALUE}>{t('common.inherit', { value: formatContextWindow(inheritedValue) })}</SelectItem>
            )}
            <SelectItem value="200000">{t('settings.model.contextWindow200k')}</SelectItem>
            <SelectItem value={String(DEFAULT_CONTEXT_WINDOW_TOKENS)}>{t('settings.model.contextWindow400k')}</SelectItem>
            <SelectItem value="1000000">{t('settings.model.contextWindow1m')}</SelectItem>
            <SelectItem value="custom">{t('settings.model.contextWindowCustom')}</SelectItem>
          </SelectGroup>
        </SelectContent>
      </Select>
      {custom && (
        <Input
          type="number"
          min={MIN_CONTEXT_WINDOW_TOKENS}
          max={MAX_CONTEXT_WINDOW_TOKENS}
          step={1000}
          value={customValue}
          placeholder={t('settings.model.contextWindowPlaceholder')}
          onBlur={() => {
            if (customDraft === null) return
            const normalized = normalizeContextWindowDraft(customDraft)
            setCustomDraft(normalized)
            if (normalized === '') {
              onChange(null)
            } else {
              const numeric = Number(normalized)
              if (Number.isFinite(numeric)) onChange(numeric)
            }
          }}
          onChange={(e) => {
            const raw = e.target.value
            setCustomDraft(raw)
            if (raw.trim() === '') return
            const numeric = Number(raw)
            if (Number.isFinite(numeric) && numeric >= MIN_CONTEXT_WINDOW_TOKENS && numeric <= MAX_CONTEXT_WINDOW_TOKENS) {
              onChange(Math.trunc(numeric))
            }
          }}
          className="sm:max-w-40"
        />
      )}
    </div>
  )
}

function normalizeContextWindowDraft(value: string) {
  const trimmed = value.trim()
  if (trimmed === '') return ''
  const numeric = Number(trimmed)
  if (!Number.isFinite(numeric)) return trimmed
  return String(Math.min(Math.max(Math.trunc(numeric), MIN_CONTEXT_WINDOW_TOKENS), MAX_CONTEXT_WINDOW_TOKENS))
}

function formatContextWindow(value: number) {
  if (value >= 1000000 && value % 1000000 === 0) return `${value / 1000000}M`
  if (value >= 1000 && value % 1000 === 0) return `${value / 1000}K`
  return String(value)
}
