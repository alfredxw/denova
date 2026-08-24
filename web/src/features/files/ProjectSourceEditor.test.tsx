import { act, render } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createRef } from 'react'
import type { ProjectFileDocument } from '@/lib/api-client/project-files'
import {
  ProjectSourceEditor,
  ProjectTextEditor,
  type ProjectTextEditorHandle,
  projectFileModelPath,
} from './ProjectSourceEditor'

const { editorProps } = vi.hoisted(() => ({ editorProps: [] as Array<Record<string, unknown>> }))

vi.mock('@monaco-editor/react', () => ({
  DiffEditor: () => null,
  Editor: (props: Record<string, unknown>) => {
    editorProps.push(props)
    return <div data-testid="monaco-editor" />
  },
}))

const document: ProjectFileDocument = {
  project_id: 'project one',
  path: 'src/main file.ts',
  content: 'before\n',
  revision: 'r1',
  kind: 'text',
  mime_type: 'text/typescript',
  size: 7,
  editable: true,
}

describe('ProjectSourceEditor', () => {
  beforeEach(() => editorProps.splice(0))

  it('keeps Monaco model-driven while drafts change and updates only real editor options', () => {
    const common = {
      projectId: 'project one',
      document,
      onWordWrapToggle: vi.fn(),
      onChange: vi.fn(),
      onSave: vi.fn(),
    }
    const { rerender } = render(<ProjectSourceEditor {...common} value={'before\n'} wordWrap />)
    const initial = editorProps.at(-1)!

    rerender(<ProjectSourceEditor {...common} value={'locally edited\n'} wordWrap />)
    const edited = editorProps.at(-1)!

    expect(initial).not.toHaveProperty('value')
    expect(initial.defaultValue).toBe('before\n')
    expect(edited.defaultValue).toBe('before\n')
    expect(edited.options).toBe(initial.options)
    expect(edited.options).toMatchObject({
      bracketPairColorization: { enabled: true },
      experimentalGpuAcceleration: 'off',
      glyphMargin: true,
      guides: {
        bracketPairs: 'active',
        indentation: true,
      },
      largeFileOptimizations: true,
      minimap: { enabled: true },
      renderLineHighlight: 'line',
      smoothScrolling: false,
      stickyScroll: { enabled: true },
      unicodeHighlight: {
        nonBasicASCII: false,
        invisibleCharacters: true,
        ambiguousCharacters: true,
        allowedLocales: {
          'zh-hans': true,
          'zh-hant': true,
          ja: true,
        },
      },
      wordWrap: 'on',
      wrappingStrategy: 'simple',
    })

    const setValue = vi.fn()
    const addAction = vi.fn(() => ({ dispose: vi.fn() }))
    ;(edited.onMount as (editor: unknown, monaco: unknown) => void)({
      getModel: () => ({
        uri: { toString: () => projectFileModelPath('project one', document.path) },
        getValue: () => 'before\n',
        setValue,
      }),
      addAction,
      onDidChangeModel: vi.fn(() => ({ dispose: vi.fn() })),
    }, {
      KeyMod: { CtrlCmd: 1, Alt: 2 },
      KeyCode: { KeyS: 3, KeyZ: 4 },
    })
    expect(setValue).toHaveBeenCalledWith('locally edited\n')

    rerender(<ProjectSourceEditor {...common} value={'locally edited\n'} wordWrap={false} />)
    expect(editorProps.at(-1)?.options).toMatchObject({ wordWrap: 'off' })
  })

  it('seeds a newly selected file model with that file content', () => {
    const common = {
      projectId: 'project one',
      wordWrap: true,
      onWordWrapToggle: vi.fn(),
      onChange: vi.fn(),
      onSave: vi.fn(),
    }
    const { rerender } = render(
      <ProjectSourceEditor {...common} document={document} value={'before\n'} />,
    )

    const nextDocument: ProjectFileDocument = {
      ...document,
      path: 'src/second file.ts',
      content: 'content from second file\n',
      revision: 'r2',
    }
    rerender(
      <ProjectSourceEditor
        {...common}
        document={nextDocument}
        value={'content from second file\n'}
      />,
    )

    expect(editorProps.at(-1)).toMatchObject({
      path: projectFileModelPath('project one', 'src/second file.ts'),
      defaultValue: 'content from second file\n',
    })
  })

  it('waits for Monaco to switch models before synchronizing the selected file content', () => {
    const firstPath = projectFileModelPath('project one', document.path)
    const secondDocument: ProjectFileDocument = {
      ...document,
      path: 'src/second file.ts',
      content: 'content from second file\n',
      revision: 'r2',
    }
    const secondPath = projectFileModelPath('project one', secondDocument.path)
    const firstSetValue = vi.fn()
    const secondSetValue = vi.fn()
    const firstModel = {
      uri: { toString: () => firstPath },
      getValue: () => 'before\n',
      setValue: firstSetValue,
      getLineCount: () => 20,
    }
    const secondModel = {
      uri: { toString: () => secondPath },
      getValue: () => 'stale second content\n',
      setValue: secondSetValue,
      getLineCount: () => 20,
    }
    let currentModel = firstModel
    let notifyModelChange = () => {}
    const editor = {
      getModel: () => currentModel,
      addAction: vi.fn(() => ({ dispose: vi.fn() })),
      onDidChangeModel: vi.fn((listener: () => void) => {
        notifyModelChange = listener
        return { dispose: vi.fn() }
      }),
    }
    const common = {
      projectId: 'project one',
      wordWrap: true,
      onWordWrapToggle: vi.fn(),
      onChange: vi.fn(),
      onSave: vi.fn(),
    }
    const { rerender } = render(
      <ProjectSourceEditor {...common} document={document} value={'before\n'} />,
    )
    act(() => {
      ;(editorProps.at(-1)?.onMount as (editor: unknown, monaco: unknown) => void)(editor, {
        KeyMod: { CtrlCmd: 1, Alt: 2 },
        KeyCode: { KeyS: 3, KeyZ: 4 },
      })
    })

    rerender(
      <ProjectSourceEditor
        {...common}
        document={secondDocument}
        value={'content from second file\n'}
      />,
    )

    expect(firstSetValue).not.toHaveBeenCalledWith('content from second file\n')
    act(() => {
      currentModel = secondModel
      notifyModelChange()
    })
    expect(secondSetValue).toHaveBeenCalledWith('content from second file\n')
  })

  it('creates an encoded, project-scoped model URI', () => {
    expect(projectFileModelPath('project one', 'src/main file.ts')).toBe(
      'file:///denova-project/project%20one/src/main%20file.ts',
    )
  })

  it('ignores Monaco model flushes while forwarding real user edits', () => {
    const onChange = vi.fn()
    render(
      <ProjectSourceEditor
        projectId="project one"
        document={document}
        value="before\n"
        wordWrap
        onWordWrapToggle={vi.fn()}
        onChange={onChange}
        onSave={vi.fn()}
      />,
    )

    const handleChange = editorProps.at(-1)?.onChange as (value: string, event: { isFlush: boolean }) => void
    handleChange('hydrated by Monaco\n', { isFlush: true })
    expect(onChange).not.toHaveBeenCalled()

    handleChange('typed by the user\n', { isFlush: false })
    expect(onChange).toHaveBeenCalledWith('typed by the user\n')
  })

  it('queues a requested line reveal until Monaco is mounted', () => {
    const ref = createRef<ProjectTextEditorHandle>()
    render(
      <ProjectTextEditor
        ref={ref}
        projectId="project one"
        document={document}
        value="before\n"
        wordWrap
        onWordWrapToggle={vi.fn()}
        onChange={vi.fn()}
        onSave={vi.fn()}
      />,
    )

    act(() => ref.current?.revealLine(7))

    const revealLineInCenter = vi.fn()
    const setPosition = vi.fn()
    const focus = vi.fn()
    const addAction = vi.fn(() => ({ dispose: vi.fn() }))
    act(() => {
      ;(editorProps.at(-1)?.onMount as (editor: unknown, monaco: unknown) => void)({
        getModel: () => ({
          uri: { toString: () => projectFileModelPath('project one', document.path) },
          getValue: () => 'before\n',
          setValue: vi.fn(),
          getLineCount: () => 20,
        }),
        addAction,
        onDidChangeModel: vi.fn(() => ({ dispose: vi.fn() })),
        revealLineInCenter,
        setPosition,
        focus,
      }, {
        KeyMod: { CtrlCmd: 1, Alt: 2 },
        KeyCode: { KeyS: 3, KeyZ: 4 },
      })
    })

    expect(revealLineInCenter).toHaveBeenCalledWith(7)
    expect(setPosition).toHaveBeenCalledWith({ lineNumber: 7, column: 1 })
    expect(focus).toHaveBeenCalled()
  })
})
