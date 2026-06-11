import common from './zh-CN/common'
import chat from './zh-CN/chat'
import editor from './zh-CN/editor'
import runtime from './zh-CN/runtime'
import inlineError from './zh-CN/inlineError'
import search from './zh-CN/search'
import sidebar from './zh-CN/sidebar'
import tab from './zh-CN/tab'
import command from './zh-CN/command'
import home from './zh-CN/home'
import importCard from './zh-CN/importCard'
import novelImport from './zh-CN/novelImport'
import router from './zh-CN/router'
import planning from './zh-CN/planning'
import lore from './zh-CN/lore'
import locale from './zh-CN/locale'
import layout from './zh-CN/layout'
import agents from './zh-CN/agents'
import settingPanel from './zh-CN/settingPanel'
import loreInit from './zh-CN/loreInit'
import writingAgent from './zh-CN/writingAgent'
import tellerPicker from './zh-CN/tellerPicker'
import storyPicker from './zh-CN/storyPicker'
import branchTimeline from './zh-CN/branchTimeline'
import storyStage from './zh-CN/storyStage'
import snapshot from './zh-CN/snapshot'
import settings from './zh-CN/settings'
import time from './zh-CN/time'
import versions from './zh-CN/versions'
import workbench from './zh-CN/workbench'
import interactiveLayout from './zh-CN/interactiveLayout'
import skills from './zh-CN/skills'
import automations from './zh-CN/automations'

const zhCN = {
  ...common,
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
  ...settings,
  ...time,
  ...versions,
  ...workbench,
  ...interactiveLayout,
  ...skills,
  ...automations,
} as const

export default zhCN
