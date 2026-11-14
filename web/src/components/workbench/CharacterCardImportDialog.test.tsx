import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { RefObject } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  CharacterCardImportDialog,
  type CharacterCardTargetMode,
} from './CharacterCardImportDialog'
import type { CharacterCardPreview, ImportDuplicatePreview } from '@/lib/api'

const preview: CharacterCardPreview = {
  name: 'Hero Card',
  entry_count: 2,
  tags: [],
  opening_preset_count: 0,
  user_placeholder_found: false,
  will_import_cover: false,
  compatibility: {
    capabilities: [],
    sanitized_runtime: [],
    discarded_extensions: [],
    warnings: [],
    ignored_loading_rules: false,
  },
  enabled_entry_count: 2,
  disabled_entry_count: 0,
  resident_entry_count: 0,
  resident_entry_bytes: 0,
  resident_lore_bytes: 0,
  auto_entry_count: 2,
  removed_runtime_entry_count: 0,
  sanitized_mixed_entry_count: 0,
  opening_truncated_count: 0,
  resident_lore_warning: false,
  resident_lore_warning_threshold_kb: 16,
  classification_mode: 'heuristic',
  classification_counts: { character: 2 },
  uncertain_type_count: 0,
}

function renderDialog(overrides: {
  pendingOverrides?: ImportDuplicatePreview[]
  notice?: string
  targetMode?: CharacterCardTargetMode
  onImport?: () => void
  onConfirmOverrides?: (choices: Record<string, string>) => void
  onCancelOverrides?: () => void
}) {
  const onOpenChange = vi.fn()
  const onConfirmOverrides = overrides.onConfirmOverrides ?? vi.fn()
  const onCancelOverrides = overrides.onCancelOverrides ?? vi.fn()
  const onImport = overrides.onImport ?? vi.fn()
  render(
    <CharacterCardImportDialog
      open
      workspace="/workspace"
      currentBookName="测试书"
      novaDir="/nova"
      file={new File(['card'], 'hero.png', { type: 'image/png' })}
      preview={preview}
      targetMode={overrides.targetMode ?? 'current'}
      bookTitle=""
      userCharacterName=""
      semanticClassification
      previewing={false}
      importing={false}
      error=""
      fileInputRef={{ current: null } as RefObject<HTMLInputElement | null>}
      onOpenChange={onOpenChange}
      onFileSelected={vi.fn()}
      onTargetModeChange={vi.fn()}
      onBookTitleChange={vi.fn()}
      onUserCharacterNameChange={vi.fn()}
      onSemanticClassificationChange={vi.fn()}
      onImport={onImport}
      pendingOverrides={overrides.pendingOverrides ?? []}
      notice={overrides.notice}
      onConfirmOverrides={onConfirmOverrides}
      onCancelOverrides={onCancelOverrides}
    />,
  )
  return { onOpenChange, onConfirmOverrides, onCancelOverrides, onImport }
}

const pendingOverrides: ImportDuplicatePreview[] = [
  {
    name: 'Hero',
    existing_preview: '1 field(s) · Short',
    incoming_preview: '5 field(s) · A much longer and more detailed description.',
    suggested: 'override',
  },
]

describe('CharacterCardImportDialog', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('shows the same-name review list with both content digests when pending', () => {
    renderDialog({ pendingOverrides })
    expect(screen.getByText('同名条目确认')).toBeInTheDocument()
    expect(screen.getByText('Hero')).toBeInTheDocument()
    expect(screen.getByText('1 field(s) · Short')).toBeInTheDocument()
    expect(screen.getByText('5 field(s) · A much longer and more detailed description.')).toBeInTheDocument()
  })

  it('preselects the suggested choice on both toggle buttons', () => {
    renderDialog({ pendingOverrides })
    expect(screen.getByRole('button', { name: '覆盖' })).toHaveAttribute('aria-pressed', 'true')
    expect(screen.getByRole('button', { name: '保留' })).toHaveAttribute('aria-pressed', 'false')
  })

  it('switches to keep and confirms with the chosen mapping', async () => {
    const user = userEvent.setup()
    const { onConfirmOverrides } = renderDialog({ pendingOverrides })
    await user.click(screen.getByRole('button', { name: '保留' }))
    expect(screen.getByRole('button', { name: '保留' })).toHaveAttribute('aria-pressed', 'true')
    expect(screen.getByRole('button', { name: '覆盖' })).toHaveAttribute('aria-pressed', 'false')
    await user.click(screen.getByRole('button', { name: '覆盖并导入' }))
    expect(onConfirmOverrides).toHaveBeenCalledWith({ Hero: 'keep' })
  })

  it('confirms with the suggested override mapping when untouched', async () => {
    const user = userEvent.setup()
    const { onConfirmOverrides } = renderDialog({ pendingOverrides })
    await user.click(screen.getByRole('button', { name: '覆盖并导入' }))
    expect(onConfirmOverrides).toHaveBeenCalledWith({ Hero: 'override' })
  })

  it('backs out of the review without importing', async () => {
    const user = userEvent.setup()
    const { onCancelOverrides, onImport } = renderDialog({ pendingOverrides })
    await user.click(screen.getByRole('button', { name: '返回' }))
    expect(onCancelOverrides).toHaveBeenCalledTimes(1)
    expect(onImport).not.toHaveBeenCalled()
  })

  it('shows a localized review-change notice when provided', () => {
    renderDialog({ pendingOverrides, notice: '部分条目在预览后已变更，请重新确认后再导入' })
    expect(screen.getByText('部分条目在预览后已变更，请重新确认后再导入')).toBeInTheDocument()
  })

  it('renders without a notice when unchanged', () => {
    renderDialog({ pendingOverrides })
    const block = screen.getByText('同名条目确认').closest('div')
    expect(block?.textContent).not.toContain('已变更')
  })

  it('keeps the plain import button without pending overrides', async () => {
    const user = userEvent.setup()
    const { onImport } = renderDialog({})
    await user.click(screen.getByRole('button', { name: '导入' }))
    expect(onImport).toHaveBeenCalledTimes(1)
  })
})