/** 方案预设目录的选中编排。 */
import type { Dispatch, SetStateAction } from 'react'
import type { PresetResourceKind } from '../../preset-ownership'
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
    setPresetResourceKind('teller')
    setActiveTellerId(id)
    closeDirectory()
  }

  const selectPresetResource = async (kind: Exclude<PresetResourceKind, 'teller'>, id: string) => {
    const activeId = currentActivePresetId(kind)
    if (presetResourceKind === kind && activeId === id) {
      closeDirectory()
      return
    }
    if (!(await flushPresetResourceAutoSave())) return
    setPresetResourceKind(kind)
    setActivePresetId(kind, id)
    closeDirectory()
  }

  const handleSelectDirectoryEntry = (id: string) => {
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
