import { fireEvent, render, screen } from '@testing-library/react'
import { createRef } from 'react'
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
  resident_lore_warning: true,
  resident_lore_warning_threshold_kb: 32,
  classification_mode: 'heuristic',
  classification_counts: { character: 2, other: 10 },
  uncertain_type_count: 10,
  compatibility: {
    capabilities: ['character_lore', 'resident_lore', 'on_demand_lore', 'narrative_openings'],
    sanitized_runtime: ['worldbook_runtime'],
    discarded_extensions: ['regex', 'mvu', 'helper'],
    warnings: [],
    ignored_loading_rules: true,
  },
}

function Harness({ cardPreview = preview, onImport = vi.fn() }: { cardPreview?: CharacterCardPreview; onImport?: () => void }) {
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
      semanticClassification
      previewing={false}
      importing={false}
      error=""
      fileInputRef={createRef<HTMLInputElement>()}
      onOpenChange={vi.fn()}
      onFileSelected={vi.fn()}
      onTargetModeChange={vi.fn()}
      onBookTitleChange={vi.fn()}
      onUserCharacterNameChange={vi.fn()}
      onSemanticClassificationChange={vi.fn()}
      onImport={onImport}
    />
  )
}

describe('CharacterCardImportDialog', () => {
  it('shows a non-blocking resident lore warning and imports normally', () => {
    const onImport = vi.fn()
    render(<Harness onImport={onImport} />)

    expect(screen.getByText('启用 326 项')).toBeInTheDocument()
    expect(screen.getByText('常驻 85 项 / 已启用 96 KB')).toBeInTheDocument()
    expect(screen.getByText('酒馆专属加载条件已忽略；关键词仅保留用于资料搜索。')).toBeInTheDocument()
    expect(screen.getByText('将语义分析用于 10 个名称无法确定类型的条目；关闭后仅使用本地名称规则。')).toBeInTheDocument()

    expect(screen.getByText('常驻资料约 107 KB，超过 32 KB 建议值。导入不会受阻；如需减少上下文占用，可在导入后将部分资料改为按需加载。')).toBeInTheDocument()
    const importButton = screen.getByRole('button', { name: '导入' })
    expect(importButton).toBeEnabled()
    fireEvent.click(importButton)
    expect(onImport).toHaveBeenCalledOnce()
  })

})
