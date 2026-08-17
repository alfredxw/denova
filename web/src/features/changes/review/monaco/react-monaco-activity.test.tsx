import { Activity } from 'react'
import { cleanup, render, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const harness = vi.hoisted(() => {
  interface FakeModel {
    uri: string
    value: string
    disposed: boolean
    dispose: () => void
    getFullModelRange: () => Record<string, never>
    setValue: (value: string) => void
  }

  interface FakeCodeEditor {
    disposed: boolean
    dispose: () => void
    getModel: () => FakeModel | null
    setModel: (model: FakeModel | null) => void
    getOption: () => boolean
    getValue: () => string
    setValue: (value: string) => void
    updateOptions: ReturnType<typeof vi.fn>
    executeEdits: ReturnType<typeof vi.fn>
    pushUndoStop: ReturnType<typeof vi.fn>
    saveViewState: ReturnType<typeof vi.fn>
    restoreViewState: ReturnType<typeof vi.fn>
    revealLine: ReturnType<typeof vi.fn>
    onDidChangeModelContent: ReturnType<typeof vi.fn>
  }

  const models = new Map<string, FakeModel>()
  const codeEditors: FakeCodeEditor[] = []
  const diffEditors: Array<{
    disposed: boolean
    dispose: () => void
    getOriginalEditor: () => FakeCodeEditor
    getModifiedEditor: () => FakeCodeEditor
    getModel: () => { original: FakeModel | null; modified: FakeModel | null }
    setModel: (models: { original: FakeModel; modified: FakeModel }) => void
    updateOptions: ReturnType<typeof vi.fn>
  }> = []

  const createModel = (value: string, _language: string, uri = `memory://${models.size}`): FakeModel => {
    const model: FakeModel = {
      uri,
      value,
      disposed: false,
      dispose: vi.fn(() => {
        model.disposed = true
        models.delete(uri)
      }),
      getFullModelRange: () => ({}),
      setValue: (nextValue) => { model.value = nextValue },
    }
    models.set(uri, model)
    return model
  }

  const createCodeEditor = (initialModel: FakeModel | null = null): FakeCodeEditor => {
    let model = initialModel
    const editor: FakeCodeEditor = {
      disposed: false,
      dispose: vi.fn(() => { editor.disposed = true }),
      getModel: () => model,
      setModel: (nextModel) => {
        if (editor.disposed) throw new Error('InstantiationService has been disposed')
        model = nextModel
      },
      getOption: () => true,
      getValue: () => model?.value ?? '',
      setValue: (value) => model?.setValue(value),
      updateOptions: vi.fn(),
      executeEdits: vi.fn(),
      pushUndoStop: vi.fn(),
      saveViewState: vi.fn(() => null),
      restoreViewState: vi.fn(),
      revealLine: vi.fn(),
      onDidChangeModelContent: vi.fn(() => ({ dispose: vi.fn() })),
    }
    codeEditors.push(editor)
    return editor
  }

  const monaco = {
    Uri: { parse: (path: string) => path },
    editor: {
      EditorOption: { readOnly: 'readOnly' },
      getModel: (uri: string) => models.get(uri),
      createModel,
      create: vi.fn((_container: HTMLElement, options: { model?: FakeModel }) => createCodeEditor(options.model)),
      createDiffEditor: vi.fn(() => {
        const original = createCodeEditor()
        const modified = createCodeEditor()
        const diffEditor = {
          disposed: false,
          dispose: vi.fn(() => {
            diffEditor.disposed = true
            original.dispose()
            modified.dispose()
          }),
          getOriginalEditor: () => original,
          getModifiedEditor: () => modified,
          getModel: () => ({ original: original.getModel(), modified: modified.getModel() }),
          setModel: (nextModels: { original: FakeModel; modified: FakeModel }) => {
            original.setModel(nextModels.original)
            modified.setModel(nextModels.modified)
          },
          updateOptions: vi.fn(),
        }
        diffEditors.push(diffEditor)
        return diffEditor
      }),
      setModelLanguage: vi.fn(),
      setTheme: vi.fn(),
      onDidChangeMarkers: vi.fn(() => ({ dispose: vi.fn() })),
      getModelMarkers: vi.fn(() => []),
    },
  }

  return {
    codeEditors,
    diffEditors,
    monaco,
    reset() {
      models.clear()
      codeEditors.splice(0)
      diffEditors.splice(0)
      monaco.editor.create.mockClear()
      monaco.editor.createDiffEditor.mockClear()
    },
  }
})

import { DiffEditor, Editor, loader } from '@monaco-editor/react'

loader.config({ monaco: harness.monaco as never })

beforeEach(() => harness.reset())
afterEach(() => cleanup())

describe('@monaco-editor/react Activity lifecycle patch', () => {
  it('recreates a diff editor after React disconnects and reconnects its effects', async () => {
    const view = render(
      <Activity mode="visible">
        <DiffEditor
          original="before"
          modified="after"
          originalModelPath="review://before"
          modifiedModelPath="review://after"
          keepCurrentOriginalModel
          keepCurrentModifiedModel
        />
      </Activity>,
    )

    await waitFor(() => expect(harness.monaco.editor.createDiffEditor).toHaveBeenCalledTimes(1))
    view.rerender(
      <Activity mode="hidden">
        <DiffEditor
          original="before"
          modified="after"
          originalModelPath="review://before"
          modifiedModelPath="review://after"
          keepCurrentOriginalModel
          keepCurrentModifiedModel
        />
      </Activity>,
    )
    await waitFor(() => expect(harness.diffEditors[0]?.dispose).toHaveBeenCalledOnce())

    view.rerender(
      <Activity mode="visible">
        <DiffEditor
          original="before"
          modified="after"
          originalModelPath="review://before"
          modifiedModelPath="review://after"
          keepCurrentOriginalModel
          keepCurrentModifiedModel
        />
      </Activity>,
    )

    await waitFor(() => expect(harness.monaco.editor.createDiffEditor).toHaveBeenCalledTimes(2))
    expect(harness.diffEditors[1]?.disposed).toBe(false)
  })

  it('recreates a standalone editor after React disconnects and reconnects its effects', async () => {
    const view = render(
      <Activity mode="visible">
        <Editor value="content" path="review://unified" keepCurrentModel />
      </Activity>,
    )

    await waitFor(() => expect(harness.monaco.editor.create).toHaveBeenCalledTimes(1))
    view.rerender(
      <Activity mode="hidden">
        <Editor value="content" path="review://unified" keepCurrentModel />
      </Activity>,
    )
    await waitFor(() => expect(harness.codeEditors[0]?.dispose).toHaveBeenCalledOnce())

    view.rerender(
      <Activity mode="visible">
        <Editor value="content" path="review://unified" keepCurrentModel />
      </Activity>,
    )

    await waitFor(() => expect(harness.monaco.editor.create).toHaveBeenCalledTimes(2))
    expect(harness.codeEditors[1]?.disposed).toBe(false)
  })
})
