import { describe, expect, it } from 'vitest'
import { isComposingKeyboardEvent, isEditableTarget, isNativeTextEditingShortcut } from './keyboard'

describe('keyboard helpers', () => {
  it('识别表单和 contenteditable 输入态', () => {
    const input = document.createElement('input')
    const textarea = document.createElement('textarea')
    const editor = document.createElement('div')
    const child = document.createElement('span')
    const plain = document.createElement('button')

    editor.contentEditable = 'true'
    editor.appendChild(child)

    expect(isEditableTarget(input)).toBe(true)
    expect(isEditableTarget(textarea)).toBe(true)
    expect(isEditableTarget(editor)).toBe(true)
    expect(isEditableTarget(child)).toBe(true)
    expect(isEditableTarget(plain)).toBe(false)
    expect(isEditableTarget(null)).toBe(false)
  })

  it('识别原生文本操作快捷键', () => {
    expect(isNativeTextEditingShortcut({ key: 'a', metaKey: true, ctrlKey: false, altKey: false })).toBe(true)
    expect(isNativeTextEditingShortcut({ key: 'V', metaKey: false, ctrlKey: true, altKey: false })).toBe(true)
    expect(isNativeTextEditingShortcut({ key: 'k', metaKey: true, ctrlKey: false, altKey: false })).toBe(false)
    expect(isNativeTextEditingShortcut({ key: 'a', metaKey: true, ctrlKey: false, altKey: true })).toBe(false)
  })

  it('识别输入法组合态按键事件', () => {
    expect(isComposingKeyboardEvent({ isComposing: true })).toBe(true)
    expect(isComposingKeyboardEvent({ nativeEvent: { isComposing: true } })).toBe(true)
    expect(isComposingKeyboardEvent({ keyCode: 229 })).toBe(true)
    expect(isComposingKeyboardEvent({ nativeEvent: { keyCode: 229 } })).toBe(true)
    expect(isComposingKeyboardEvent({ keyCode: 13 })).toBe(false)
  })
})
