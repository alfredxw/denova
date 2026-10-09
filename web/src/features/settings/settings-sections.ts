import type { LucideIcon } from 'lucide-react'
import { BookOpen, Bot, Folder, Globe, History, Image, Languages, Settings2, ShieldCheck, Store, Terminal, Upload, Volume2, Wifi } from 'lucide-react'

export const MODEL_ENDPOINT_FIELD_KEYS: readonly string[] = ['settings.model.endpointAliasLabel', 'settings.model.profileProviderLabel', 'settings.model.profileProtocolLabel', 'common.baseUrl', 'settings.model.profileKeyLabel', 'settings.model.sessionKeyMappingLabel', 'settings.model.sessionKeyFieldLabel']
export const MODEL_PROFILE_FIELD_KEYS: readonly string[] = ['settings.model.profileModelLabel', 'settings.model.profileAliasLabel', 'settings.model.profileTemperatureLabel', 'settings.model.maxTokens', 'settings.model.contextWindow']
export const IMAGE_ENDPOINT_FIELD_KEYS: readonly string[] = ['settings.imageApi.endpointAliasLabel', 'settings.imageApi.provider', 'settings.imageApi.protocol', 'common.baseUrl', 'settings.imageApi.profileKeyLabel']

type SettingsGroup = 'preferences' | 'creation' | 'models' | 'tools' | 'system'
type SettingsSectionDefinition = {
  id: string
  group: SettingsGroup
  titleKey: string
  icon: LucideIcon
  fieldKeys: readonly string[]
}

/** Navigation and search share one ordered definition; groups do not change a setting's scope. */
export const SETTINGS_SECTIONS = [
  { id: 'general', group: 'preferences', titleKey: 'settings.section.general', icon: Languages,
    fieldKeys: ['settings.appearance.language', 'settings.appearance.theme', 'settings.appearance.motionIntensity', 'settings.appearance.uiFont', 'settings.appearance.uiFontSize', 'settings.appearance.readingFont', 'settings.appearance.readingFontSize', 'settings.appearance.sourceEditorFont', 'settings.onboarding.title'] },
  { id: 'writing', group: 'creation', titleKey: 'settings.section.writing', icon: BookOpen,
    fieldKeys: ['settings.ide.autoSave', 'settings.ide.autoSaveInterval', 'settings.ide.chapterFilenameFormat', 'settings.ide.volumeDirFormat', 'settings.ide.maxOpenTabs', 'settings.ide.chapterGroupMin', 'settings.ide.chapterGroupMax', 'settings.ide.defaultTeller', 'settings.agent.writingSkillDefault'] },
  { id: 'model', group: 'models', titleKey: 'settings.section.model', icon: Bot,
    fieldKeys: ['settings.model.defaultProfile', ...MODEL_ENDPOINT_FIELD_KEYS, ...MODEL_PROFILE_FIELD_KEYS] },
  { id: 'image', group: 'models', titleKey: 'settings.section.imageApi', icon: Image,
    fieldKeys: ['settings.imageApi.defaultProfile', ...IMAGE_ENDPOINT_FIELD_KEYS, 'settings.imageApi.profileModelLabel', 'settings.imageApi.profileAliasLabel', 'settings.imageApi.promptGuide', 'settings.imageApi.defaultSize', 'settings.imageApi.defaultAspectRatio', 'settings.imageApi.defaultResolution', 'settings.imageApi.defaultQuality', 'settings.imageApi.defaultOutputFormat'] },
  { id: 'speech', group: 'models', titleKey: 'speech.title', icon: Volume2,
    fieldKeys: ['speech.endpoint', 'speech.apiKey', 'speech.model', 'speech.voice'] },
  { id: 'approvals', group: 'tools', titleKey: 'settings.section.approvals', icon: ShieldCheck,
    fieldKeys: ['settings.agent.approvalMode', 'agentApproval.rules.title'] },
  { id: 'terminal', group: 'tools', titleKey: 'settings.section.terminal', icon: Terminal,
    fieldKeys: ['settings.terminal.enabled', 'settings.terminal.shell', 'settings.terminal.commandName', 'settings.terminal.launchCommand', 'settings.terminal.maxSessions', 'settings.terminal.scrollbackKB', 'settings.agent.shellEnvironmentMode', 'settings.agent.shellEnvironmentShell', 'settings.agent.bashPath'] },
  { id: 'web-access', group: 'tools', titleKey: 'settings.section.webAccess', icon: Globe,
    fieldKeys: ['settings.webAccess.searxngBaseUrl', 'settings.webAccess.searchMaxResults', 'settings.webAccess.searchProviderTimeoutSeconds', 'settings.webAccess.fetchMaxResponseKB', 'settings.webAccess.fetchMaxContentChars'] },
  { id: 'market', group: 'tools', titleKey: 'settings.section.market', icon: Store,
    fieldKeys: ['settings.market.registryURL'] },
  { id: 'paths', group: 'system', titleKey: 'settings.section.paths', icon: Folder,
    fieldKeys: ['settings.paths.skillsDir', 'settings.paths.novaDir', 'settings.paths.userConfig'] },
  { id: 'versions', group: 'system', titleKey: 'settings.section.versions', icon: History,
    fieldKeys: ['settings.versions.timedAuto', 'settings.versions.timedInterval'] },
  { id: 'access', group: 'system', titleKey: 'settings.section.access', icon: Wifi,
    fieldKeys: ['settings.access.allowLan', 'settings.access.username', 'settings.access.password'] },
  { id: 'updates', group: 'system', titleKey: 'settings.section.updates', icon: Upload,
    fieldKeys: ['settings.updates.autoCheck', 'settings.updates.manual', 'settings.updates.check'] },
  { id: 'advanced', group: 'system', titleKey: 'settings.section.advanced', icon: Settings2,
    fieldKeys: ['settings.agent.maxIteration', 'settings.agent.modelMaxRetries', 'settings.agent.idleTimeoutSeconds', 'settings.agent.toolResultLimitKB', 'settings.agent.toolParallelism', 'settings.agent.subAgentParallelism', 'settings.agent.scriptTimeoutSeconds', 'settings.agent.planModeDefault', 'settings.labs.developerMode'] },
] as const satisfies readonly SettingsSectionDefinition[]

export type SettingsSectionId = (typeof SETTINGS_SECTIONS)[number]['id']

/** A transient navigation request also reveals fields inside collapsed editors. */
export interface SettingsFieldRequest {
  id: SettingsSectionId
  fieldKey?: string
}
