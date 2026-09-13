import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { readProjectFile, saveProjectFile, type ProjectFileDocument } from '@/lib/api-client/project-files'
import { SourceManifestEditor } from './SourceManifestEditor'

vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key, i18n: { language: 'en-US' } }) }))
vi.mock('@/lib/api-client/project-files', () => ({ readProjectFile: vi.fn(), saveProjectFile: vi.fn() }))

afterEach(() => { cleanup(); vi.resetAllMocks() })

const manifest = { id: 'test.game', version: '1.0.0', name: { 'zh-CN': '花园', 'en-US': 'Garden' }, permissions: { required: ['gameData'] }, game: { viewId: 'stage', storage: { kind: 'self', saveFormat: 'garden-v1' } }, development: { build: { command: 'node', args: ['--check', 'game.mjs'] } } }
const document: ProjectFileDocument = { project_id: 'project', path: 'denova.game.json', content: JSON.stringify(manifest), revision: 'r1', kind: 'text', mime_type: 'application/json', editable: true, size: 200 }
const props = { source: { developmentId: 'project', projectId: 'project', projectName: 'Garden', relativePath: '.', kind: 'game' as const }, onSaved: vi.fn(), onOpenFile: vi.fn(), onFlushChange: vi.fn() }

describe('source manifest editor', () => {
  it('finishes the first read when a project refresh arrives during loading', async () => {
    let finish!: (value: ProjectFileDocument) => void
    vi.mocked(readProjectFile).mockReturnValueOnce(new Promise(resolve => { finish = resolve })).mockResolvedValue(document)
    const view = render(<SourceManifestEditor {...props} refreshSignal={0} />)
    view.rerender(<SourceManifestEditor {...props} refreshSignal={1} />)
    expect(readProjectFile).toHaveBeenCalledTimes(1)
    await act(async () => finish(document))
    await waitFor(() => expect(screen.getByLabelText('platform.extensionName')).toHaveValue('Garden'))
    expect(screen.queryByText('common.loading')).not.toBeInTheDocument()
  })

  it('saves the displayed language while preserving the other language and advanced source fields', async () => {
    vi.mocked(readProjectFile).mockResolvedValue(document)
    vi.mocked(saveProjectFile).mockResolvedValue({ project_id: 'project', path: document.path, revision: 'r2', changed: true })
    render(<SourceManifestEditor {...props} refreshSignal={0} />)
    await waitFor(() => expect(screen.getByLabelText('platform.extensionName')).toHaveValue('Garden'))
    fireEvent.change(screen.getByLabelText('platform.extensionName'), { target: { value: 'Garden revised' } })
    fireEvent.change(screen.getByLabelText('platform.sourceCover'), { target: { value: 'images/cover.png' } })
    const flush = props.onFlushChange.mock.lastCall![0]
    await act(async () => { expect(await flush()).toBe(true) })
    const [project, path, content, revision] = vi.mocked(saveProjectFile).mock.lastCall!
    expect([project, path, revision]).toEqual(['project', 'denova.game.json', 'r1'])
    expect(JSON.parse(content)).toEqual({ ...manifest, name: { ...manifest.name, 'en-US': 'Garden revised' }, game: { ...manifest.game, cover: 'images/cover.png' } })
  })
})
