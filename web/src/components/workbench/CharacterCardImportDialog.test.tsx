import { fireEvent, render, screen } from '@testing-library/react'
import { createRef, useState } from 'react'
import { describe, expect, it, vi } from 'vitest'
import type { CharacterCardPreview } from '@/lib/api'
import { CharacterCardImportDialog } from './CharacterCardImportDialog'

const preview: CharacterCardPreview = {
  name: '命定之诗',
  entry_count: 469,
  tags: [],
  opening_preset_count: 3,
  user_placeholder_found: false,
  will_import_cover: true,
  enabled_entry_count: 326,
  disabled_entry_count: 143,
  resident_entry_count: 85,
  resident_entry_bytes: 96 * 1024,
  resident_lore_bytes: 107 * 1024,
  auto_entry_count: 373,
  removed_runtime_entry_count: 11,
  sanitized_mixed_entry_count: 73,
  opening_truncated_count: 0,
  current_resident_lore_bytes: 0,
  resident_lore_limit_kb: 32,
  max_resident_lore_limit_kb: 1024,
  required_current_resident_lore_limit_kb: 107,
  required_new_book_resident_lore_limit_kb: 107,
  compatibility: {
    capabilities: ['character_lore', 'resident_lore', 'on_demand_lore', 'narrative_openings'],
    sanitized_runtime: ['worldbook_runtime'],
    discarded_extensions: ['regex', 'mvu', 'helper'],
    warnings: [],
    ignored_loading_rules: true,
  },
}

function Harness({ cardPreview = preview }: { cardPreview?: CharacterCardPreview }) {
  const [raiseLimit, setRaiseLimit] = useState(false)
  return (
    <CharacterCardImportDialog
      open
      workspace="/tmp/book"
      currentBookName="当前作品"
      novaDir="/tmp"
      file={new File(['card'], 'card.png', { type: 'image/png' })}
      preview={cardPreview}
      targetMode="new_book"
      bookTitle="命定之诗"
      userCharacterName=""
      raiseResidentLoreLimit={raiseLimit}
      previewing={false}
      importing={false}
      error=""
      fileInputRef={createRef<HTMLInputElement>()}
      onOpenChange={vi.fn()}
      onFileSelected={vi.fn()}
      onTargetModeChange={vi.fn()}
      onBookTitleChange={vi.fn()}
      onUserCharacterNameChange={vi.fn()}
      onRaiseResidentLoreLimitChange={setRaiseLimit}
      onImport={vi.fn()}
    />
  )
}

describe('CharacterCardImportDialog', () => {
  it('shows native import stats and requires explicit resident budget confirmation', () => {
    render(<Harness />)

    expect(screen.getByText('启用 326 项')).toBeInTheDocument()
    expect(screen.getByText('常驻 85 项 / 已启用 96 KB')).toBeInTheDocument()
    expect(screen.getByText('酒馆专属加载条件已忽略；关键词仅保留用于资料搜索。')).toBeInTheDocument()

    const importButton = screen.getByRole('button', { name: '导入' })
    expect(importButton).toBeDisabled()
    fireEvent.click(screen.getByRole('checkbox'))
    expect(importButton).toBeEnabled()
  })

  it('rejects a card whose resident lore exceeds the hard maximum', () => {
    render(<Harness cardPreview={{ ...preview, required_new_book_resident_lore_limit_kb: 1025 }} />)

    expect(screen.queryByRole('checkbox')).not.toBeInTheDocument()
    expect(screen.getByText('常驻资料需要 1025 KB，超过最大上限 1024 KB，无法导入。')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '导入' })).toBeDisabled()
  })
})
