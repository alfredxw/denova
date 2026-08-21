/** 方案预设目录的选中编排。 */
import type { Dispatch, SetStateAction } from 'react'
import type { PresetResourceKind } from '../../preset-ownership'
import { TELLER_CONFIG_AGENT_ENTRY_ID } from './presetResources'
import { parsePresetDirectoryEntryId } from './preset-directory-sections'

/**
 * 目录条目选择：切换前冲刷 autosave，并维护各资源编辑器的当前选中项。
 */
export function usePresetSelection({
  presetResourceKind,
  setPresetResourceKind,
  activeTellerId,
  setActiveTellerId,
  currentActivePresetId,
  setActivePresetId,
  flushPresetResourceAutoSave,
  closeDirectory,
}: {
  presetResourceKind: PresetResourceKind
  setPresetResourceKind: (kind: PresetResourceKind) => void
  activeTellerId: string
  setActiveTellerId: Dispatch<SetStateAction<string>>
  currentActivePresetId: (kind: PresetResourceKind) => string
  setActivePresetId: (kind: Exclude<PresetResourceKind, 'teller'>, id: string) => void
  flushPresetResourceAutoSave: () => Promise<boolean>
  closeDirectory: () => void
}) {
  const handleSelectTeller = async (id: string) => {
    if (presetResourceKind === 'teller' && activeTellerId === id) {
      closeDirectory()
      return
    }
    if (!(await flushPresetResourceAutoSave())) return
    if (id !== TELLER_CONFIG_AGENT_ENTRY_ID) {
      setPresetResourceKind('teller')
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

  return { handleSelectTeller, selectPresetResource, handleSelectDirectoryEntry }
}
