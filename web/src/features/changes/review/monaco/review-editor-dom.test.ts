import { describe, expect, it, vi } from 'vitest'
import type { editor } from 'monaco-editor'
import { fitReviewEditorToHost, installReviewEditorPointerFocus } from './review-editor-dom'

describe('review editor DOM integration', () => {
  it('focuses the clicked editor without browser scrolling and seeds the exact pointer position', () => {
    const root = document.createElement('div')
    const line = document.createElement('span')
    const editContext = document.createElement('div')
    editContext.className = 'native-edit-context'
    root.append(line, editContext)
    document.body.append(root)
    const position = { lineNumber: 18, column: 7 }
    const focus = vi.spyOn(editContext, 'focus')
    const codeEditor = {
      getDomNode: () => root,
      hasTextFocus: () => false,
      getTargetAtClientPoint: vi.fn(() => ({ position })),
      setPosition: vi.fn(),
    } as unknown as editor.IStandaloneCodeEditor

    const disposable = installReviewEditorPointerFocus(codeEditor)
    line.dispatchEvent(new MouseEvent('mousedown', { bubbles: true, button: 0, clientX: 320, clientY: 480 }))

    expect(codeEditor.getTargetAtClientPoint).toHaveBeenCalledWith(320, 480)
    expect(codeEditor.setPosition).toHaveBeenCalledWith(position, 'review-pointer')
    expect(focus).toHaveBeenCalledWith({ preventScroll: true })

    disposable.dispose()
    root.remove()
  })

  it('does not steal focus from inline comment controls inside a Monaco view zone', () => {
    const root = document.createElement('div')
    const zone = document.createElement('div')
    zone.className = 'nova-review-zone-host'
    const input = document.createElement('textarea')
    zone.append(input)
    root.append(zone)
    const codeEditor = {
      getDomNode: () => root,
      getTargetAtClientPoint: vi.fn(),
      setPosition: vi.fn(),
    } as unknown as editor.IStandaloneCodeEditor

    const disposable = installReviewEditorPointerFocus(codeEditor)
    input.dispatchEvent(new MouseEvent('mousedown', { bubbles: true, button: 0 }))

    expect(codeEditor.getTargetAtClientPoint).not.toHaveBeenCalled()
    disposable.dispose()
  })

  it('leaves pointer selection gestures to Monaco after the editor has text focus', () => {
    const root = document.createElement('div')
    const line = document.createElement('span')
    root.append(line)
    const codeEditor = {
      getDomNode: () => root,
      hasTextFocus: () => true,
      getTargetAtClientPoint: vi.fn(),
      setPosition: vi.fn(),
    } as unknown as editor.IStandaloneCodeEditor

    const disposable = installReviewEditorPointerFocus(codeEditor)
    line.dispatchEvent(new MouseEvent('mousedown', { bubbles: true, button: 0, clientX: 320, clientY: 480 }))

    expect(codeEditor.getTargetAtClientPoint).not.toHaveBeenCalled()
    expect(codeEditor.setPosition).not.toHaveBeenCalled()
    disposable.dispose()
  })

  it('lays out lazily mounted editors from the already-sized host instead of Monaco\'s 5px fallback', () => {
    const host = document.createElement('div')
    Object.defineProperties(host, {
      clientWidth: { configurable: true, value: 640 },
      clientHeight: { configurable: true, value: 480 },
    })
    const layout = vi.fn()
    let resize: ResizeObserverCallback | undefined
    const ResizeObserverHarness = class {
      constructor(callback: ResizeObserverCallback) { resize = callback }
      observe() {}
      disconnect() {}
    }
    vi.stubGlobal('ResizeObserver', ResizeObserverHarness)

    const disposable = fitReviewEditorToHost({ layout }, host)

    expect(layout).toHaveBeenCalledWith({ width: 640, height: 480 })
    resize?.([{ target: host } as unknown as ResizeObserverEntry], {} as ResizeObserver)
    expect(layout).toHaveBeenLastCalledWith({ width: 640, height: 480 })

    disposable.dispose()
    vi.unstubAllGlobals()
  })
})
