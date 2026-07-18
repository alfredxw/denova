/** 方案预设的选中编排与写作/游戏双模式选中记忆（lastSelectionByMode + usageMode 变化恢复）。 */
import { useEffect, useRef, type Dispatch, type SetStateAction } from 'react'
import { presetResourceVisibleInMode, type PresetResourceKind, type PresetUsageMode } from '../../preset-ownership'
import { TELLER_CONFIG_AGENT_ENTRY_ID } from './presetResources'
import { parsePresetDirectoryEntryId } from './preset-directory-sections'

const PRESET_MODE_FALLBACK_ORDER: PresetResourceKind[] = ['teller', 'image', 'director', 'event', 'rule', 'actor-state']

type ModeSelection = { kind: PresetResourceKind; id: string }

/**
 * 目录条目选择与切模式记忆：选中前冲刷 autosave；每个 usageMode 记住各自上次选中的资源，
 * 切模式时记存旧模式选中、恢复新模式记忆，无记忆时按兜底顺序选第一个可见可用条目。
 */
export function usePresetSelection({
  workspace,
  presetUsageMode,
  presetResourceKind,
  setPresetResourceKind,
  activeTellerId,
  setActiveTellerId,
  currentActivePresetId,
  setActivePresetId,
  presetItemsForKind,
  flushPresetResourceAutoSave,
  closeDirectory,
}: {
  workspace: string
  presetUsageMode: PresetUsageMode
  presetResourceKind: PresetResourceKind
  setPresetResourceKind: (kind: PresetResourceKind) => void
  activeTellerId: string
  setActiveTellerId: Dispatch<SetStateAction<string>>
  currentActivePresetId: (kind: PresetResourceKind) => string
  setActivePresetId: (kind: Exclude<PresetResourceKind, 'teller'>, id: string) => void
  presetItemsForKind: (kind: PresetResourceKind) => { id: string }[]
  flushPresetResourceAutoSave: () => Promise<boolean>
  closeDirectory: () => void
}) {
  const lastSelectionByMode = useRef<Partial<Record<PresetUsageMode, ModeSelection>>>({})

  useEffect(() => {
    lastSelectionByMode.current = {}
  }, [workspace])

  const handleSelectTeller = async (id: string) => {
    if (presetResourceKind === 'teller' && activeTellerId === id) {
      closeDirectory()
      return
    }
    if (!(await flushPresetResourceAutoSave())) return
    if (id !== TELLER_CONFIG_AGENT_ENTRY_ID) {
      setPresetResourceKind('teller')
      lastSelectionByMode.current[presetUsageMode] = { kind: 'teller', id }
    }
    setActiveTellerId(id)
    closeDirectory()
  }

  const selectPresetResource = async (kind: Exclude<PresetResourceKind, 'teller'>, id: string) => {
    const activeId = currentActivePresetId(kind)
    if (presetResourceKind === kind && activeId === id && activeTellerId !== TELLER_CONFIG_AGENT_ENTRY_ID) {
      closeDirectory()
      return
    }
    if (!(await flushPresetResourceAutoSave())) return
    setPresetResourceKind(kind)
    lastSelectionByMode.current[presetUsageMode] = { kind, id }
    setActiveTellerId((current) => current === TELLER_CONFIG_AGENT_ENTRY_ID ? '' : current)
    setActivePresetId(kind, id)
    closeDirectory()
  }

  const handleSelectDirectoryEntry = (id: string) => {
    if (id === TELLER_CONFIG_AGENT_ENTRY_ID) {
      void handleSelectTeller(id)
      return
    }
    const parsed = parsePresetDirectoryEntryId(id)
    if (!parsed) return
    if (parsed.kind === 'teller') {
      void handleSelectTeller(parsed.itemId)
      return
    }
    void selectPresetResource(parsed.kind, parsed.itemId)
  }

  // 无依赖数组：每次渲染后检查模式是否变化，保证读到最新选中与列表闭包
  const previousPresetUsageModeRef = useRef(presetUsageMode)
  useEffect(() => {
    const previousMode = previousPresetUsageModeRef.current
    if (previousMode === presetUsageMode) return
    previousPresetUsageModeRef.current = presetUsageMode
    const configAgentActive = activeTellerId === TELLER_CONFIG_AGENT_ENTRY_ID
    const currentId = currentActivePresetId(presetResourceKind)
    if (currentId && currentId !== TELLER_CONFIG_AGENT_ENTRY_ID) {
      lastSelectionByMode.current[previousMode] = { kind: presetResourceKind, id: currentId }
    }
    // 配置管理 Agent 是跨模式伪条目，切模式时不打断 Agent 会话
    if (configAgentActive) return
    const remembered = lastSelectionByMode.current[presetUsageMode]
    if (remembered && presetResourceVisibleInMode(remembered.kind, presetUsageMode) && presetItemsForKind(remembered.kind).some((item) => item.id === remembered.id)) {
      if (remembered.kind === 'teller') void handleSelectTeller(remembered.id)
      else void selectPresetResource(remembered.kind, remembered.id)
      return
    }
    for (const kind of PRESET_MODE_FALLBACK_ORDER) {
      if (!presetResourceVisibleInMode(kind, presetUsageMode)) continue
      const first = presetItemsForKind(kind)[0]
      if (!first) continue
      if (kind === 'teller') void handleSelectTeller(first.id)
      else void selectPresetResource(kind, first.id)
      return
    }
  })

  return { handleSelectTeller, selectPresetResource, handleSelectDirectoryEntry }
}
