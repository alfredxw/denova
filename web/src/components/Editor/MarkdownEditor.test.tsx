import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { MarkdownEditor } from './MarkdownEditor'

vi.mock('@tiptap/react', () => ({
  EditorContent: () => <div data-testid="editor-content" />,
  useEditor: () => ({
    commands: {
      setContent: vi.fn(),
      focus: vi.fn(),
    },
    storage: {
      characterCount: {
        characters: () => 0,
      },
    },
    state: {
      doc: { textContent: '' },
      selection: { from: 0, to: 0, empty: true },
      tr: { setMeta: vi.fn() },
    },
    view: {
      dispatch: vi.fn(),
      dom: document.createElement('div'),
    },
    isDestroyed: false,
    getText: () => '',
    getHTML: () => '',
    on: vi.fn(),
    off: vi.fn(),
  }),
}))

vi.mock('@tiptap/starter-kit', () => ({ default: { configure: () => ({}) } }))
vi.mock('@tiptap/extension-character-count', () => ({ CharacterCount: { configure: () => ({}) } }))
vi.mock('@tiptap/extension-placeholder', () => ({ default: { configure: () => ({}) } }))
vi.mock('@tiptap/markdown', () => ({ Markdown: { configure: () => ({}) } }))

describe('MarkdownEditor', () => {
  it('打开编辑器设置 Popover 后展示行间距和背景主题', async () => {
    const user = userEvent.setup()

    render(
      <MarkdownEditor
        fileName="chapters/ch01.md"
        content="第一章"
        onSave={vi.fn()}
      />,
    )

    await user.click(screen.getByRole('button', { name: '编辑器设置' }))

    expect(screen.getByText('编辑器设置')).toBeInTheDocument()
    expect(screen.getByText('行间距')).toBeInTheDocument()
    expect(screen.getByText('背景主题')).toBeInTheDocument()
  })
})
