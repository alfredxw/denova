import { render } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { ProjectFileDocument } from './api'
import { ProjectSourceEditor, projectFileModelPath } from './ProjectSourceEditor'

const { editorProps } = vi.hoisted(() => ({ editorProps: [] as Array<Record<string, unknown>> }))

vi.mock('@monaco-editor/react', () => ({
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
      editContext: false,
      experimentalGpuAcceleration: 'off',
      guides: {
        bracketPairs: false,
        indentation: false,
      },
      largeFileOptimizations: true,
      smoothScrolling: false,
      wordWrap: 'on',
      wrappingStrategy: 'simple',
    })

    const setValue = vi.fn()
    const addAction = vi.fn(() => ({ dispose: vi.fn() }))
    ;(edited.onMount as (editor: unknown, monaco: unknown) => void)({
      getModel: () => ({ getValue: () => 'before\n', setValue }),
      addAction,
    }, {
      KeyMod: { CtrlCmd: 1, Alt: 2 },
      KeyCode: { KeyS: 3, KeyZ: 4 },
    })
    expect(setValue).toHaveBeenCalledWith('locally edited\n')

    rerender(<ProjectSourceEditor {...common} value={'locally edited\n'} wordWrap={false} />)
    expect(editorProps.at(-1)?.options).toMatchObject({ wordWrap: 'off' })
  })

  it('creates an encoded, project-scoped model URI', () => {
    expect(projectFileModelPath('project one', 'src/main file.ts')).toBe(
      'file:///denova-project/project%20one/src/main%20file.ts',
    )
  })
})
