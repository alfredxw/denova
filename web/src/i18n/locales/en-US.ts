import common from './en-US/common'
import remoteAccess from './en-US/remoteAccess'
import configManager from './en-US/configManager'
import chat from './en-US/chat'
import editor from './en-US/editor'
import runtime from './en-US/runtime'
import inlineError from './en-US/inlineError'
import search from './en-US/search'
import sidebar from './en-US/sidebar'
import tab from './en-US/tab'
import command from './en-US/command'
import home from './en-US/home'
import importCard from './en-US/importCard'
import novelImport from './en-US/novelImport'
import router from './en-US/router'
import planning from './en-US/planning'
import lore from './en-US/lore'
import locale from './en-US/locale'
import layout from './en-US/layout'
import agents from './en-US/agents'
import settingPanel from './en-US/settingPanel'
import loreInit from './en-US/loreInit'
import writingAgent from './en-US/writingAgent'
import tellerPicker from './en-US/tellerPicker'
import storyPicker from './en-US/storyPicker'
import branchTimeline from './en-US/branchTimeline'
import storyStage from './en-US/storyStage'
import snapshot from './en-US/snapshot'
import directorPanel from './en-US/directorPanel'
import settings from './en-US/settings'
import time from './en-US/time'
import versions from './en-US/versions'
import workbench from './en-US/workbench'
import interactiveLayout from './en-US/interactiveLayout'
import skills from './en-US/skills'
import automations from './en-US/automations'
import messages from './en-US/messages'
import onboarding from './en-US/onboarding'
import changes from './en-US/changes'

const enUS = {
  ...common,
  ...remoteAccess,
  ...configManager,
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
} as const

export default enUS
