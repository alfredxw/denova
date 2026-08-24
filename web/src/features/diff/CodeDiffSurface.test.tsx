import { forwardRef, useImperativeHandle } from 'react'
import { fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { CodeDiffSurface } from './CodeDiffSurface'
import type { DiffFileDocument, DiffFileNavigationItem } from './types'
import { useMultiFileDiffNavigation } from './use-multi-file-diff-navigation'

const codeViewState = vi.hoisted(() => ({
  itemIDs: [] as string[],
  options: null as null | { enableGutterUtility?: boolean; unsafeCSS?: string; layout?: { gap?: number } },
}))

vi.mock('@pierre/diffs/react', () => ({
  CodeView: forwardRef(function MockCodeView(props: { items: Array<{ id: string }>; options?: { enableGutterUtility?: boolean; unsafeCSS?: string; layout?: { gap?: number } }; className?: string; renderCustomHeader?: (item: { id: string }) => React.ReactNode }, ref) {
    codeViewState.itemIDs = props.items.map((item) => item.id)
    codeViewState.options = props.options ?? null
    useImperativeHandle(ref, () => ({
      scrollTo: vi.fn(),
      clearSelectedLines: vi.fn(),
      getInstance: () => undefined,
    }))
    return <div data-testid="code-view" className={props.className}>{props.items.map((item) => <div key={item.id}>{props.renderCustomHeader?.(item)}</div>)}</div>
  }),
}))

vi.mock('./DiffFileNavigator', () => ({
  DiffFileNavigator: ({ files, onSelect }: { files: DiffFileNavigationItem[]; onSelect: (path: string) => void }) => (
    <nav>{files.map((file) => <button key={file.path} type="button" onClick={() => onSelect(file.path)}>{file.path}</button>)}</nav>
  ),
}))

vi.mock('@/components/layout/panel-motion', () => ({
  InlineCollapsiblePane: ({ children }: { children: React.ReactNode }) => <aside>{children}</aside>,
}))

describe('CodeDiffSurface', () => {
  beforeEach(() => {
    codeViewState.itemIDs = []
    codeViewState.options = null
  })

  it('passes all small files to one virtualized CodeView', () => {
    render(<SurfaceHarness files={[file('a.ts', 'old', 'new'), file('b.ts', 'one', 'two')]} />)
    expect(codeViewState.itemIDs).toEqual(['a.ts', 'b.ts'])
    expect(codeViewState.options?.layout?.gap).toBe(0)
    expect(getComputedStyle(screen.getByTestId('code-view')).getPropertyValue('--diffs-font-family').trim()).toBe('var(--nova-source-editor-font-family)')
    expect(screen.queryByText(/Diff 较大|diff is large/i)).not.toBeInTheDocument()
  })

  it('switches large diffs to one-file mode and navigates without mounting every file', () => {
    const largeText = Array.from({ length: 3_010 }, (_, index) => `line ${index}`).join('\n')
    render(<SurfaceHarness files={[file('large.ts', '', largeText), file('next.ts', '', 'next')]} />)

    expect(codeViewState.itemIDs).toEqual(['large.ts'])
    expect(screen.getByRole('status')).toHaveTextContent(/Diff 较大|diff is large/i)
    fireEvent.click(screen.getByRole('button', { name: /下一个变更文件|Next changed file/i }))
    expect(codeViewState.itemIDs).toEqual(['next.ts'])
  })

  it('enables the hover comment utility only when the host provides review annotations', () => {
    const review = render(<SurfaceHarness files={[file('review.ts', 'old', 'new')]} onLineSelectionEnd={vi.fn()} />)
    expect(codeViewState.options?.enableGutterUtility).toBe(true)
    expect(codeViewState.options?.unsafeCSS).toContain('margin-right: 0')

    review.unmount()
    render(<SurfaceHarness files={[file('version.ts', 'old', 'new')]} />)
    expect(codeViewState.options?.enableGutterUtility).toBe(false)
    expect(codeViewState.options?.unsafeCSS).toBeUndefined()
  })

  it('orders each file header as path, diff totals, then actions', () => {
    render(<SurfaceHarness files={[file('src/review.ts', '', 'new')]} />)

    const path = screen.getByText('review.ts')
    const additions = screen.getByText('+1')
    const copy = screen.getByRole('button', { name: /复制路径|Copy path/i })
    expect(path.compareDocumentPosition(additions) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    expect(additions.compareDocumentPosition(copy) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    expect(path.closest('[data-code-diff-header]')).toHaveClass('hover:bg-[var(--nova-hover)]')
  })
})

function SurfaceHarness({ files, onLineSelectionEnd }: { files: DiffFileDocument[]; onLineSelectionEnd?: () => void }) {
  const paths = files.map((item) => item.path)
  const navigation = useMultiFileDiffNavigation({ identity: 'test', paths })
  const navigatorFiles: DiffFileNavigationItem[] = files.map((item) => ({ path: item.path, kind: 'modified' }))
  return (
    <CodeDiffSurface
      files={files}
      navigatorFiles={navigatorFiles}
      navigation={navigation}
      layout="unified"
      ariaLabel="Diff"
      empty={<div>Empty</div>}
      onLineSelectionEnd={onLineSelectionEnd}
    />
  )
}

function file(path: string, before: string, after: string): DiffFileDocument {
  return {
    path,
    before_content: before,
    after_content: after,
    base_revision: `${path}:before`,
    revision: `${path}:after`,
  }
}
