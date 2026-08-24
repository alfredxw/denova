import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ProjectWritingSurface } from './ProjectWritingSurface'

const api = vi.hoisted(() => ({
  getProjectBookSnapshot: vi.fn(),
  getProjectBookSummary: vi.fn(),
  readProjectFile: vi.fn(),
  saveProjectFile: vi.fn(),
  setProjectChapterConfirmed: vi.fn(),
}))
const projectFiles = vi.hoisted(() => ({ applyProjectFileOperations: vi.fn() }))

vi.mock('@/lib/api', async (importOriginal) => ({
  ...await importOriginal<typeof import('@/lib/api')>(),
  ...api,
}))

vi.mock('@/lib/api-client/project-files', async (importOriginal) => ({
  ...await importOriginal<typeof import('@/lib/api-client/project-files')>(),
  ...projectFiles,
}))

vi.mock('@/components/layout/adaptive-surface', () => ({
  AdaptiveSurface: ({ left, children }: {
    left: { content: React.ReactNode }
    children: (state: { isMobile: boolean; openLeft: () => void }) => React.ReactNode
  }) => <div>{left.content}{children({ isMobile: false, openLeft: vi.fn() })}</div>,
}))

vi.mock('@/components/workbench/outline/ChapterOutline', () => ({
  ChapterOutline: ({ projectId, onCreateItem, onSetChapterConfirmed }: {
    projectId: string
    onCreateItem: (path: string, type: 'file' | 'dir') => Promise<void>
    onSetChapterConfirmed: (path: string, confirmed: boolean) => Promise<void>
  }) => (
    <div>
      <button type="button" onClick={() => void onSetChapterConfirmed('chapters/ch01.md', true)}>
        Confirm {projectId}
      </button>
      <button type="button" onClick={() => void onCreateItem('chapters/ch00002-new.md', 'file')}>
        Create chapter {projectId}
      </button>
    </div>
  ),
}))

vi.mock('@/components/Editor/WritingDocumentEditor', () => ({
  WritingDocumentEditor: ({ projectId, fileName, content, revision, onSave }: {
    projectId: string
    fileName: string
    content: string
    revision: string
    onSave: (path: string, content: string, baseRevision: string) => Promise<unknown>
  }) => (
    <div>
      <span data-testid="project-document">{projectId}|{fileName}|{content}|{revision}</span>
      <button type="button" onClick={() => void onSave(fileName, 'After', revision)}>Save</button>
    </div>
  ),
}))

vi.mock('@/components/Editor/WritingSourceEditor', () => ({
  WritingSourceEditor: ({ document }: { document: { path: string; content?: string } }) => (
    <div data-testid="project-source-document">{document.path}|{document.content}</div>
  ),
}))

describe('ProjectWritingSurface', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    const summary = {
      title: 'Background',
      author: '',
      chapter_count: 1,
      total_words: 1,
      chapters: [{
        path: 'chapters/ch01.md',
        file_name: 'ch01.md',
        display_title: 'Chapter 1',
        index: 1,
        words: 1,
        status: 'draft',
        confirmed: false,
        updated_at: '',
        volume: '',
        volume_path: 'chapters',
      }],
      chapter_plans: [],
    }
    api.getProjectBookSnapshot.mockResolvedValue({
      project_id: 'book-b',
      workspace: '/canonical/books/b',
      tree: [{ name: 'chapters', type: 'dir', children: [{ name: 'ch01.md', type: 'file' }] }],
      summary,
    })
    api.getProjectBookSummary.mockResolvedValue(summary)
    api.readProjectFile.mockResolvedValue({
      project_id: 'book-b',
      path: 'chapters/ch01.md',
      content: 'Before',
      revision: 'r1',
      kind: 'text',
      mime_type: 'text/markdown',
      size: 6,
      editable: true,
    })
    api.saveProjectFile.mockResolvedValue({
      project_id: 'book-b',
      path: 'chapters/ch01.md',
      revision: 'r2',
      changed: true,
    })
    api.setProjectChapterConfirmed.mockResolvedValue({ confirmed: true })
    projectFiles.applyProjectFileOperations.mockResolvedValue([{
      kind: 'create',
      ok: true,
      path: 'chapters/ch00002-new.md',
    }])
  })

  it('creates chapters in an embedded Book without switching the foreground workspace', async () => {
    const user = userEvent.setup()
    const onWorkspaceChanged = vi.fn()
    render(
      <ProjectWritingSurface
        projectId="book-b"
        documentReview={{ comments: [], onCreate: vi.fn(), onUpdate: vi.fn(), onDelete: vi.fn() }}
        onFlushHandlerChange={vi.fn()}
        onWorkspaceChanged={onWorkspaceChanged}
      />,
    )

    await user.click(await screen.findByRole('button', { name: 'Create chapter book-b' }))

    await waitFor(() => expect(projectFiles.applyProjectFileOperations).toHaveBeenCalledWith('book-b', [{
      kind: 'create',
      path: 'chapters/ch00002-new.md',
      type: 'file',
      content: '',
    }]))
    expect(onWorkspaceChanged).toHaveBeenCalledWith(
      ['chapters/ch00002-new.md'],
      { impact: 'structure', origin: 'project-page' },
    )
  })

  it('reads, edits, and confirms a background Book through its Project ID', async () => {
    const user = userEvent.setup()
    const onWorkspaceChanged = vi.fn()
    render(
      <ProjectWritingSurface
        projectId="book-b"
        documentReview={{ comments: [], onCreate: vi.fn(), onUpdate: vi.fn(), onDelete: vi.fn() }}
        onFlushHandlerChange={vi.fn()}
        onWorkspaceChanged={onWorkspaceChanged}
      />,
    )

    expect(await screen.findByTestId('project-document')).toHaveTextContent(
      'book-b|chapters/ch01.md|Before|r1',
    )
    expect(api.getProjectBookSnapshot).toHaveBeenCalledWith('book-b')
    expect(api.readProjectFile).toHaveBeenCalledWith('book-b', 'chapters/ch01.md')

    await user.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(api.saveProjectFile).toHaveBeenCalledWith(
      'book-b',
      'chapters/ch01.md',
      'After',
      'r1',
    ))

    await user.click(screen.getByRole('button', { name: 'Confirm book-b' }))
    await waitFor(() => expect(api.setProjectChapterConfirmed).toHaveBeenCalledWith(
      'book-b',
      'chapters/ch01.md',
      true,
    ))
    expect(onWorkspaceChanged).toHaveBeenCalledWith(
      ['chapters/ch01.md'],
      { impact: 'content', origin: 'project-page' },
    )
  })

  it('opens non-Markdown text with the shared Monaco source surface', async () => {
    const snapshot = await api.getProjectBookSnapshot('book-b')
    api.getProjectBookSnapshot.mockResolvedValue({
      ...snapshot,
      tree: [{ name: 'data.json', type: 'file' }],
    })
    api.readProjectFile.mockResolvedValue({
      project_id: 'book-b',
      path: 'data.json',
      content: '{"exact":true}',
      revision: 'r-json',
      kind: 'text',
      mime_type: 'application/json',
      size: 14,
      editable: true,
    })

    render(
      <ProjectWritingSurface
        projectId="book-b"
        initialPath="data.json"
        documentReview={{ comments: [], onCreate: vi.fn(), onUpdate: vi.fn(), onDelete: vi.fn() }}
        onFlushHandlerChange={vi.fn()}
      />,
    )

    expect(await screen.findByTestId('project-source-document')).toHaveTextContent('data.json|{"exact":true}')
    expect(screen.queryByTestId('project-document')).not.toBeInTheDocument()
  })
})
