import market from './ru-RU/market'
import gameDefaults from './ru-RU/gameDefaults'
import speech from './ru-RU/speech'
import common from './ru-RU/common'
import platform from './ru-RU/platform'
import agentRuntime from './ru-RU/agentRuntime'
import remoteAccess from './ru-RU/remoteAccess'
import chat from './ru-RU/chat'
import editor from './ru-RU/editor'
import runtime from './ru-RU/runtime'
import inlineError from './ru-RU/inlineError'
import search from './ru-RU/search'
import sidebar from './ru-RU/sidebar'
import tab from './ru-RU/tab'
import command from './ru-RU/command'
import home from './ru-RU/home'
import importCard from './ru-RU/importCard'
import novelImport from './ru-RU/novelImport'
import router from './ru-RU/router'
import planning from './ru-RU/planning'
import lore from './ru-RU/lore'
import loreWorkspace from './ru-RU/loreWorkspace'
import locale from './ru-RU/locale'
import layout from './ru-RU/layout'
import agents from './ru-RU/agents'
import settingPanel from './ru-RU/settingPanel'
import loreInit from './ru-RU/loreInit'
import writingAgent from './ru-RU/writingAgent'
import tellerPicker from './ru-RU/tellerPicker'
import storyPicker from './ru-RU/storyPicker'
import branchTimeline from './ru-RU/branchTimeline'
import storyStage from './ru-RU/storyStage'
import snapshot from './ru-RU/snapshot'
import directorPanel from './ru-RU/directorPanel'
import settings from './ru-RU/settings'
import time from './ru-RU/time'
import versions from './ru-RU/versions'
import workbench from './ru-RU/workbench'
import interactiveLayout from './ru-RU/interactiveLayout'
import skills from './ru-RU/skills'
import automations from './ru-RU/automations'
import messages from './ru-RU/messages'
import onboarding from './ru-RU/onboarding'
import changes from './ru-RU/changes'
import narrativeStyle from './ru-RU/narrativeStyle'
import agentChat from './ru-RU/agentChat'
import agentApproval from './ru-RU/agentApproval'
import files from './ru-RU/files'
import trajectory from './ru-RU/trajectory'

const ruRU = {
  ...common,
  ...market,
  ...gameDefaults,
  ...platform,
  ...speech,
  ...agentRuntime,
  ...remoteAccess,
  ...chat,
  ...editor,
  ...runtime,
  ...inlineError,
  ...search,
  ...sidebar,
  ...tab,
  ...command,
  ...home,
  ...importCard,
  ...novelImport,
  ...router,
  ...planning,
  ...lore,
  ...loreWorkspace,
  ...locale,
  ...layout,
  ...agents,
  ...settingPanel,
  ...loreInit,
  ...writingAgent,
  ...tellerPicker,
  ...storyPicker,
  ...branchTimeline,
  ...storyStage,
  ...snapshot,
  ...directorPanel,
  ...settings,
  ...time,
  ...versions,
  ...workbench,
  ...interactiveLayout,
  ...skills,
  ...automations,
  ...messages,
  ...onboarding,
  ...changes,
  ...narrativeStyle,
  ...agentChat,
  ...agentApproval,
  ...files,
  ...trajectory,
  'agents.tool.availability.runtimeCheck': 'Проверка среды выполнения',
} as const

export default ruRU
