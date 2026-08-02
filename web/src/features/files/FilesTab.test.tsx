import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { useState, type ReactNode } from 'react'
import { describe, expect, it, vi } from 'vitest'
import { server } from '@/test/msw/server'
import { FilesTab } from './FilesTab'

vi.mock('@monaco-editor/react', () => ({
  Editor: ({ value, onChange, options }: {
    value: string
    onChange: (value: string) => void
    options?: { ariaLabel?: string }
  }) => (
    <textarea
      aria-label={options?.ariaLabel}
      value={value}
      onChange={(event) => onChange(event.target.value)}
    />
  ),
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
}: {
  initialPath?: string | null
  onWorkspaceChanged?: (workspace: string, paths: string[]) => void
}) {
  const [selectedPath, setSelectedPath] = useState<string | null>(initialPath)
  return (
    <FilesTab
      projectId="project-one"
      workspace="/projects/one"
      selectedPath={selectedPath}
      autoSaveEnabled={false}
      autoSaveDelayMs={50}
      onSelectedPathChange={setSelectedPath}
      onWorkspaceChanged={onWorkspaceChanged}
    />
  )
}

describe('FilesTab', () => {
  it('loads directories lazily and saves Monaco edits with the selected revision', async () => {
    const user = userEvent.setup()
    const onWorkspaceChanged = vi.fn()
    let savedBody: unknown
    server.use(
      http.get('/api/projects/project-one/files', ({ request }) => {
        const path = new URL(request.url).searchParams.get('path') || ''
        return HttpResponse.json(path === 'src'
          ? {
              project_id: 'project-one',
              path,
              entries: [{ name: 'main.ts', path: 'src/main.ts', type: 'file', modified_at: '2026-08-02T00:00:00Z' }],
            }
          : {
              project_id: 'project-one',
              path: '',
              entries: [{ name: 'src', path: 'src', type: 'dir', modified_at: '2026-08-02T00:00:00Z' }],
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
    await user.click(await screen.findByRole('button', { name: 'src' }))
    await user.click(await screen.findByRole('button', { name: 'main.ts' }))

    const editor = await screen.findByRole('textbox', { name: 'src/main.ts 的源码编辑器' })
    fireEvent.change(editor, { target: { value: 'after\n' } })
    await user.click(screen.getByRole('button', { name: '保存文件' }))

    await waitFor(() => expect(savedBody).toEqual({
      path: 'src/main.ts',
      content: 'after\n',
      base_revision: 'r1',
    }))
    await waitFor(() => expect(onWorkspaceChanged).toHaveBeenCalledWith('/projects/one', ['src/main.ts']))
  })

  it('rebases a stale save over the latest external content before retrying', async () => {
    const user = userEvent.setup()
    let reads = 0
    const saves: Array<Record<string, unknown>> = []
    server.use(
      http.get('/api/projects/project-one/files', () => HttpResponse.json({ project_id: 'project-one', path: '', entries: [] })),
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
})
