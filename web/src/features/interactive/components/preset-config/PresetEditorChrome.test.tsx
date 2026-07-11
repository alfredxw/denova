import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { PresetMetadataPanel } from './PresetEditorChrome'

describe('PresetMetadataPanel', () => {
  it('keeps preset identity compact without exposing resource tags', () => {
    render(
      <PresetMetadataPanel
        name="均衡 DM 检定"
        description="通用均衡裁定风格"
        status="内置"
        onNameChange={vi.fn()}
        onDescriptionChange={vi.fn()}
      />,
    )

    expect(screen.getByRole('textbox', { name: '名称' })).toBeInTheDocument()
    expect(screen.getByRole('textbox', { name: '描述' })).toBeInTheDocument()
    expect(screen.queryByRole('textbox', { name: '标签' })).not.toBeInTheDocument()
  })
})
