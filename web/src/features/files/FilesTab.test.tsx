import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { useEffect, useState, type ReactNode } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { WorkspaceChangeMetadata } from '@/features/changes/types'
import { fileTreeRow } from '@/test/file-tree'
import { server } from '@/test/msw/server'
import { TestQueryClientProvider } from '@/test/query-client'
import { FilesTab } from './FilesTab'

vi.mock('@monaco-editor/react', () => ({
  DiffEditor: () => null,
  Editor: ({ defaultValue, onChange, options }: {
    defaultValue: string
    onChange: (value: string, event?: { isFlush: boolean }) => void
    options?: { ariaLabel?: string }
  }) => {
    const [value, setValue] = useState(defaultValue)
    useEffect(() => {
      onChange(defaultValue, { isFlush: true })
    }, [defaultValue, onChange])
    return (
      <textarea
        aria-label={options?.ariaLabel}
        value={value}
        onChange={(event) => {
          setValue(event.target.value)
          onChange(event.target.value)
        }}
      />
    )
  },
}))

vi.mock('@/components/layout/adaptive-surface', () => ({
  AdaptiveSurface: ({ right, children }: {
    right?: { content: ReactNode; desktopVisible?: boolean }
    children: ReactNode | ((controls: {
      isMobile: boolean
      openRight: () => void
    }) => ReactNode)
  }) => (
    <div>
      <main>{typeof children === 'function' ? children({ isMobile: false, openRight: vi.fn() }) : children}</main>
      {right?.desktopVisible === false ? null : <aside>{right?.content}</aside>}
    </div>
  ),
}))

function FilesHarness({
  initialPath = null,
  onWorkspaceChanged = vi.fn(),
  autoSaveEnabled = false,
  editorRefreshSignal = 0,
  treeRefreshSignal = 0,
}: {
  initialPath?: string | null
  onWorkspaceChanged?: (workspace: string, paths: string[], metadata: WorkspaceChangeMetadata) => void
  autoSaveEnabled?: boolean
  editorRefreshSignal?: number
  treeRefreshSignal?: number
}) {
  const [selectedPath, setSelectedPath] = useState<string | null>(initialPath)
  return (
    <TestQueryClientProvider>
      <FilesTab
        projectId="project-one"
        workspace="/projects/one"
        selectedPath={selectedPath}
        autoSaveEnabled={autoSaveEnabled}
        autoSaveDelayMs={50}
        editorRefreshSignal={editorRefreshSignal}
        treeRefreshSignal={treeRefreshSignal}
        onSelectedPathChange={setSelectedPath}
        onWorkspaceChanged={onWorkspaceChanged}
      />
    </TestQueryClientProvider>
  )
}

describe('FilesTab', () => {
  beforeEach(() => window.localStorage.clear())

  it('recursively resolves the directory tree in one request and saves Monaco edits with the selected revision', async () => {
    const user = userEvent.setup()
    const onWorkspaceChanged = vi.fn()
    let savedBody: unknown
    server.use(
      http.post('/api/projects/project-one/files/resolve', async ({ request }) => {
        expect(await request.json()).toMatchObject({
          targets: [{ path: '' }],
          recursive: true,
        })
        return HttpResponse.json({
          project_id: 'project-one',
          results: [{
            path: '',
            ok: true,
            directories: [
              { path: '', revision: 'tree-root', entries: [{ name: 'src', path: 'src', type: 'dir' }], children_state: 'complete' },
              { path: 'src', revision: 'tree-src', entries: [{ name: 'main.ts', path: 'src/main.ts', type: 'file' }], children_state: 'complete' },
            ],
          }],
        })
      }),
      http.get('/api/projects/project-one/files/file', () => HttpResponse.json({
        project_id: 'project-one',
        path: 'src/main.ts',
        content: 'before\n',
        revision: 'r1',
        kind: 'text',
        mime_type: 'text/typescript',
        size: 7,
        editable: true,
      })),
      http.put('/api/projects/project-one/files/file', async ({ request }) => {
        savedBody = await request.json()
        return HttpResponse.json({ project_id: 'project-one', path: 'src/main.ts', revision: 'r2', changed: true })
      }),
    )

    render(<FilesHarness onWorkspaceChanged={onWorkspaceChanged} />)
    await waitFor(() => expect(fileTreeRow('src/')).toBeInTheDocument())
    await user.click(fileTreeRow('src/'))
    await user.click(fileTreeRow('src/main.ts'))

    const editor = await screen.findByRole('textbox', { name: 'src/main.ts 的源码编辑器' })
    fireEvent.change(editor, { target: { value: 'after\n' } })
    await user.click(screen.getByRole('button', { name: '保存文件' }))

    await waitFor(() => expect(savedBody).toEqual({
      path: 'src/main.ts',
      content: 'after\n',
      base_revision: 'r1',
    }))
    await waitFor(() => expect(onWorkspaceChanged).toHaveBeenCalledWith('/projects/one', ['src/main.ts'], {
      impact: 'content',
      origin: 'files-tab',
    }))
  })

  it('rebases a stale save over the latest external content before retrying', async () => {
    const user = userEvent.setup()
    let reads = 0
    const saves: Array<Record<string, unknown>> = []
    server.use(
      http.post('/api/projects/project-one/files/resolve', async ({ request }) => {
        const body = await request.json() as { targets: Array<{ path: string }> }
        return HttpResponse.json({
          project_id: 'project-one',
          results: body.targets.map((target) => ({
            path: target.path,
            ok: true,
            directories: [{ path: target.path, revision: `tree-${target.path || 'root'}`, entries: [], children_state: 'complete' }],
          })),
        })
      }),
      http.get('/api/projects/project-one/files/file', () => {
        reads += 1
        return HttpResponse.json({
          project_id: 'project-one',
          path: 'src/main.ts',
          content: reads === 1 ? 'alpha\nmiddle\nbeta\n' : 'alpha\nmiddle\nbeta external\n',
          revision: reads === 1 ? 'r1' : 'r2',
          kind: 'text',
          mime_type: 'text/typescript',
          size: 11,
          editable: true,
        })
      }),
      http.put('/api/projects/project-one/files/file', async ({ request }) => {
        const body = await request.json() as Record<string, unknown>
        saves.push(body)
        if (saves.length === 1) {
          return HttpResponse.json({ error: 'stale', code: 'revision_conflict' }, { status: 409 })
        }
        return HttpResponse.json({ project_id: 'project-one', path: 'src/main.ts', revision: 'r3', changed: true })
      }),
    )

    render(<FilesHarness initialPath="src/main.ts" />)
    const editor = await screen.findByRole('textbox', { name: 'src/main.ts 的源码编辑器' })
    fireEvent.change(editor, { target: { value: 'alpha local\nmiddle\nbeta\n' } })
    await user.click(screen.getByRole('button', { name: '保存文件' }))

    await waitFor(() => expect(saves).toHaveLength(2))
    expect(saves[0]).toMatchObject({ content: 'alpha local\nmiddle\nbeta\n', base_revision: 'r1' })
    expect(saves[1]).toMatchObject({ content: 'alpha local\nmiddle\nbeta external\n', base_revision: 'r2' })
  })

  it('opens Markdown in preview and persists the source word-wrap preference', async () => {
    const user = userEvent.setup()
    server.use(
      http.post('/api/projects/project-one/files/resolve', () => HttpResponse.json({
        project_id: 'project-one',
        results: [{
          path: '',
          ok: true,
          directories: [{
            path: '',
            revision: 'tree-root',
            entries: [{ name: 'README.md', path: 'README.md', type: 'file' }],
            children_state: 'complete',
          }],
        }],
      })),
      http.get('/api/projects/project-one/files/file', () => HttpResponse.json({
        project_id: 'project-one',
        path: 'README.md',
        content: '# Hello Files\n',
        revision: 'r1',
        kind: 'text',
        mime_type: 'text/markdown',
        size: 14,
        editable: true,
      })),
    )

    render(<FilesHarness initialPath="README.md" />)
    expect(await screen.findByRole('heading', { name: 'Hello Files' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '预览' })).toHaveAttribute('aria-pressed', 'true')

    await user.click(screen.getByRole('button', { name: '源码' }))
    expect(await screen.findByRole('textbox', { name: 'README.md 的源码编辑器' })).toHaveValue('# Hello Files\n')
    const wrap = screen.getByRole('button', { name: '关闭自动换行' })
    expect(wrap).toHaveAttribute('aria-pressed', 'true')
    await user.click(wrap)
    expect(screen.getByRole('button', { name: '启用自动换行' })).toHaveAttribute('aria-pressed', 'false')
    expect(JSON.parse(window.localStorage.getItem('nova.project-file-editor.preferences.v1') ?? '{}')).toEqual({ wordWrap: false })
  })

  it('reloads external content without resolving loaded directories again', async () => {
    let resolveRequests = 0
    let fileReads = 0
    server.use(
      http.post('/api/projects/project-one/files/resolve', async ({ request }) => {
        resolveRequests += 1
        const body = await request.json() as { targets: Array<{ path: string }> }
        return HttpResponse.json({
          project_id: 'project-one',
          results: body.targets.map((target) => ({
            path: target.path,
            ok: true,
            directories: [{
              path: target.path,
              revision: `tree-${resolveRequests}-${target.path || 'root'}`,
              entries: target.path === '' ? [{ name: 'main.ts', path: 'main.ts', type: 'file' }] : [],
              children_state: 'complete',
            }],
          })),
        })
      }),
      http.get('/api/projects/project-one/files/file', () => {
        fileReads += 1
        return HttpResponse.json({
          project_id: 'project-one',
          path: 'main.ts',
          content: `revision ${fileReads}`,
          revision: `r${fileReads}`,
          kind: 'text',
          mime_type: 'text/typescript',
          size: 10,
          editable: true,
        })
      }),
    )

    const { rerender } = render(<FilesHarness initialPath="main.ts" />)
    await waitFor(() => expect(fileReads).toBe(1))
    await waitFor(() => expect(resolveRequests).toBe(1))

    rerender(<FilesHarness initialPath="main.ts" editorRefreshSignal={1} />)
    await waitFor(() => expect(fileReads).toBe(2))
    expect(resolveRequests).toBe(1)

    rerender(<FilesHarness initialPath="main.ts" editorRefreshSignal={1} treeRefreshSignal={1} />)
    await waitFor(() => expect(resolveRequests).toBe(2))
  })

  it('does not autosave when a file is only opened and Monaco hydrates its model', async () => {
    let saves = 0
    server.use(
      http.post('/api/projects/project-one/files/resolve', () => HttpResponse.json({
        project_id: 'project-one',
        results: [{
          path: '',
          ok: true,
          directories: [{ path: '', revision: 'tree-root', entries: [], children_state: 'complete' }],
        }],
      })),
      http.get('/api/projects/project-one/files/file', () => HttpResponse.json({
        project_id: 'project-one',
        path: 'main.ts',
        content: 'unchanged\n',
        revision: 'r1',
        kind: 'text',
        mime_type: 'text/typescript',
        size: 10,
        editable: true,
      })),
      http.put('/api/projects/project-one/files/file', () => {
        saves += 1
        return HttpResponse.json({ project_id: 'project-one', path: 'main.ts', revision: 'r2', changed: false })
      }),
    )

    render(<FilesHarness initialPath="main.ts" autoSaveEnabled />)
    await screen.findByRole('textbox', { name: 'main.ts 的源码编辑器' })
    await new Promise((resolve) => window.setTimeout(resolve, 100))

    expect(saves).toBe(0)
    expect(screen.getByRole('button', { name: '保存文件' })).toBeDisabled()
  })
})
