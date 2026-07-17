import { forwardRef, useCallback, useEffect, useImperativeHandle, useMemo, useRef, useState, type CSSProperties } from 'react'
import { EditorContent, useEditor } from '@tiptap/react'
import { Node, mergeAttributes } from '@tiptap/core'
import StarterKit from '@tiptap/starter-kit'
import { TextSelection } from '@tiptap/pm/state'
import type { Editor } from '@tiptap/react'
import type { Node as ProseMirrorNode } from '@tiptap/pm/model'

type ComposerTokenKind = 'skill' | 'file' | 'lore' | 'style'
type ComposerTriggerKind = 'slash' | 'reference' | 'style'

export interface ComposerTokenSpec {
  kind: ComposerTokenKind
  value: string
  label?: string
}

export interface ComposerTrigger {
  kind: ComposerTriggerKind
  query: string
}

export interface ComposerTokenInputHandle {
  focus: () => void
  select: () => void
  setSelectionRange: (start: number, end: number) => void
  replaceActiveTriggerText: (text: string) => void
  replaceActiveTriggerWithToken: (token: ComposerTokenSpec) => void
  ensureTokens: (tokens: ComposerTokenSpec[]) => void
  deleteToLineStart: () => void
  deleteCurrentLine: () => void
  selectCurrentWord: () => void
  getValue: () => string
}

interface ComposerTokenInputProps {
  value: string
  onChange: (value: string) => void
  onTriggerChange?: (trigger: ComposerTrigger | null) => void
  onTokenRemove?: (token: ComposerTokenSpec) => void
  onEditorKeyDown?: (event: KeyboardEvent) => boolean
  knownSkills?: string[]
  knownFiles?: string[]
  knownLore?: Array<{ id: string; label: string }>
  knownStyleScenes?: string[]
  externalTokens?: ComposerTokenSpec[]
  placeholder?: string
  disabled?: boolean
  rows?: number
  minRows?: number
  maxRows?: number
  multilineMode?: 'auto' | 'sticky-until-empty' | 'always'
  inputMode?: string
  enterKeyHint?: string
  autoCapitalize?: string
  className?: string
  style?: CSSProperties
}

type TokenEntry = ComposerTokenSpec & { key: string }
type ActiveTriggerRange = ComposerTrigger & { from: number; to: number }

const tokenExtension = Node.create({
  name: 'composerToken',
  group: 'inline',
  inline: true,
  atom: true,
  selectable: true,

  addAttributes() {
    return {
      kind: { default: 'file' },
      value: { default: '' },
      label: { default: '' },
    }
  },

  parseHTML() {
    return [{ tag: 'span[data-nova-composer-token]' }]
  },

  renderHTML({ HTMLAttributes }) {
    const kind = normalizeTokenKind(HTMLAttributes.kind)
    const value = String(HTMLAttributes.value || '')
    const label = String(HTMLAttributes.label || value)
    return [
      'span',
      mergeAttributes(HTMLAttributes, {
        'data-nova-composer-token': kind,
        'data-token-value': value,
        class: `nova-composer-token nova-composer-token-${kind}`,
      }),
      tokenDisplayText({ kind, value, label }),
    ]
  },
})

const composerExtensions = [
  StarterKit.configure({
    blockquote: false,
    bulletList: false,
    code: false,
    codeBlock: false,
    dropcursor: false,
    gapcursor: false,
    heading: false,
    horizontalRule: false,
    listItem: false,
    orderedList: false,
  }),
  tokenExtension,
]

let textMeasureCanvas: HTMLCanvasElement | null = null

export const ComposerTokenInput = forwardRef<ComposerTokenInputHandle, ComposerTokenInputProps>(function ComposerTokenInput({
  value,
  onChange,
  onTriggerChange,
  onTokenRemove,
  onEditorKeyDown,
  knownSkills = [],
  knownFiles = [],
  knownLore = [],
  knownStyleScenes = [],
  externalTokens = [],
  placeholder = '',
  disabled = false,
  rows = 1,
  minRows = 1,
  maxRows = 10,
  multilineMode = 'auto',
  inputMode,
  enterKeyHint,
  autoCapitalize,
  className,
  style,
}, ref) {
  const rootRef = useRef<HTMLDivElement | null>(null)
  const onChangeRef = useRef(onChange)
  const onTriggerChangeRef = useRef(onTriggerChange)
  const onTokenRemoveRef = useRef(onTokenRemove)
  const onEditorKeyDownRef = useRef(onEditorKeyDown)
  const disabledRef = useRef(disabled)
  const activeTriggerRef = useRef<ActiveTriggerRange | null>(null)
  const previousTokenKeysRef = useRef<Set<string>>(new Set())
  const compactTextWidthRef = useRef(0)
  const multilineRef = useRef(false)
  const [multiline, setMultiline] = useState(() => multilineMode === 'always')
  const [empty, setEmpty] = useState(() => value.length === 0)

  const parseOptions = useMemo<ComposerParseOptions>(() => ({
    skills: uniqueStrings(knownSkills),
    files: uniqueStrings(knownFiles),
    lore: knownLore.filter((item) => item.id && item.label),
    styleScenes: uniqueStrings(knownStyleScenes),
  }), [knownFiles, knownLore, knownSkills, knownStyleScenes])

  const editor = useEditor({
    extensions: composerExtensions,
    content: textToComposerJSON(value, parseOptions),
    editable: !disabled,
    editorProps: {
      attributes: {
        role: 'textbox',
        'aria-multiline': 'true',
        rows: String(rows),
        placeholder,
        inputmode: inputMode || '',
        enterkeyhint: enterKeyHint || '',
        autocapitalize: autoCapitalize || '',
        class: 'nova-composer-token-editor',
      },
      handleKeyDown: (_view, event) => {
        if (onEditorKeyDownRef.current?.(event)) return true
        if (disabledRef.current) return false
        return handleDefaultKeyDown(event)
      },
    },
    onUpdate: ({ editor: nextEditor }) => {
      const nextValue = serializeComposerDoc(nextEditor.state.doc)
      onChangeRef.current(nextValue)
      emitTrigger(nextEditor)
      syncEmptyAndTokens(nextEditor)
      requestAnimationFrame(syncHeight)
    },
    onSelectionUpdate: ({ editor: nextEditor }) => {
      emitTrigger(nextEditor)
    },
  })

  useEffect(() => {
    onChangeRef.current = onChange
  }, [onChange])

  useEffect(() => {
    onTriggerChangeRef.current = onTriggerChange
  }, [onTriggerChange])

  useEffect(() => {
    onTokenRemoveRef.current = onTokenRemove
  }, [onTokenRemove])

  useEffect(() => {
    onEditorKeyDownRef.current = onEditorKeyDown
  }, [onEditorKeyDown])

  useEffect(() => {
    disabledRef.current = disabled
    if (!editor || editor.isDestroyed) return
    editor.setEditable(!disabled)
    const dom = editor.view.dom
    dom.setAttribute('aria-disabled', disabled ? 'true' : 'false')
    if (disabled) dom.setAttribute('contenteditable', 'false')
  }, [disabled, editor])

  useEffect(() => {
    if (!editor || editor.isDestroyed) return
    const dom = editor.view.dom
    dom.setAttribute('rows', String(rows))
    dom.setAttribute('placeholder', placeholder)
    if (inputMode) dom.setAttribute('inputmode', inputMode)
    else dom.removeAttribute('inputmode')
    if (enterKeyHint) dom.setAttribute('enterkeyhint', enterKeyHint)
    else dom.removeAttribute('enterkeyhint')
    if (autoCapitalize) dom.setAttribute('autocapitalize', autoCapitalize)
    else dom.removeAttribute('autocapitalize')
  }, [autoCapitalize, editor, enterKeyHint, inputMode, placeholder, rows])

  useEffect(() => {
    if (!editor || editor.isDestroyed) return
    const current = serializeComposerDoc(editor.state.doc)
    if (current === value) {
      setEmpty(value.length === 0)
      requestAnimationFrame(syncHeight)
      return
    }
    if (value === '') {
      editor.commands.clearContent({ emitUpdate: false })
    } else {
      editor.commands.setContent(textToComposerJSON(value, parseOptions), { emitUpdate: false })
    }
    previousTokenKeysRef.current = new Set(collectTokenEntries(editor.state.doc).map((token) => token.key))
    emitTrigger(editor)
    syncEmptyAndTokens(editor, false)
    requestAnimationFrame(syncHeight)
  }, [editor, parseOptions, value])

  useEffect(() => {
    if (!editor || externalTokens.length === 0) return
    ensureTokens(editor, externalTokens)
    requestAnimationFrame(syncHeight)
  }, [editor, externalTokens])

  useEffect(() => {
    requestAnimationFrame(syncHeight)
  }, [editor, maxRows, minRows, multilineMode, value])

  useEffect(() => {
    if (!editor) return
    previousTokenKeysRef.current = new Set(collectTokenEntries(editor.state.doc).map((token) => token.key))
    syncEmptyAndTokens(editor, false)
    requestAnimationFrame(syncHeight)
  }, [editor])

  useImperativeHandle(ref, () => ({
    focus: () => editor?.commands.focus(),
    select: () => {
      if (!editor) return
      editor.commands.focus()
      editor.commands.setTextSelection({ from: 1, to: editor.state.doc.content.size })
    },
    setSelectionRange: (start, end) => {
      if (!editor) return
      const from = textOffsetToDocPosition(editor.state.doc, start)
      const to = textOffsetToDocPosition(editor.state.doc, end)
      editor.view.dispatch(editor.state.tr.setSelection(TextSelection.create(editor.state.doc, from, to)))
      editor.commands.focus()
    },
    replaceActiveTriggerText: (text) => {
      if (!editor) return
      replaceActiveTrigger(editor, text)
    },
    replaceActiveTriggerWithToken: (token) => {
      if (!editor) return
      replaceActiveTrigger(editor, tokenToInsertContent(token))
    },
    ensureTokens: (tokens) => {
      if (!editor) return
      ensureTokens(editor, tokens)
    },
    deleteToLineStart: () => {
      if (!editor) return
      const { from } = selectionTextOffsets(editor)
      const text = serializeComposerDoc(editor.state.doc)
      const lineStart = text.lastIndexOf('\n', Math.max(0, from - 1)) + 1
      replaceTextRange(editor, lineStart, from, '')
    },
    deleteCurrentLine: () => {
      if (!editor) return
      const { from } = selectionTextOffsets(editor)
      const text = serializeComposerDoc(editor.state.doc)
      const lineStart = text.lastIndexOf('\n', Math.max(0, from - 1)) + 1
      let lineEnd = text.indexOf('\n', from)
      if (lineEnd === -1) lineEnd = text.length
      else lineEnd += 1
      replaceTextRange(editor, lineStart, lineEnd, '')
    },
    selectCurrentWord: () => {
      if (!editor) return
      const text = serializeComposerDoc(editor.state.doc)
      const { from } = selectionTextOffsets(editor)
      const wordBoundary = /[\s,.:;!?'\"(){}[\]@#$%^&*+=<>/\\|~`\-]/
      let start = from
      while (start > 0 && !wordBoundary.test(text[start - 1])) start--
      let end = from
      while (end < text.length && !wordBoundary.test(text[end])) end++
      const docFrom = textOffsetToDocPosition(editor.state.doc, start)
      const docTo = textOffsetToDocPosition(editor.state.doc, end)
      editor.view.dispatch(editor.state.tr.setSelection(TextSelection.create(editor.state.doc, docFrom, docTo)))
      editor.commands.focus()
    },
    getValue: () => editor ? serializeComposerDoc(editor.state.doc) : value,
  }), [editor, parseOptions, value])

  const syncHeight = useCallback(() => {
    const element = rootRef.current
    if (!element) return
    const computed = window.getComputedStyle(element)
    const lineHeight = parseCssPixels(computed.lineHeight) || 20
    const paddingTop = parseCssPixels(computed.paddingTop)
    const paddingBottom = parseCssPixels(computed.paddingBottom)
    const borderTop = parseCssPixels(computed.borderTopWidth)
    const borderBottom = parseCssPixels(computed.borderBottomWidth)
    const minHeight = parseCssPixels(computed.minHeight)
    const verticalChrome = paddingTop + paddingBottom + borderTop + borderBottom
    const minRowCount = Math.max(1, minRows)
    const maxRowCount = Math.max(minRowCount, maxRows)
    const oneRowHeight = Math.ceil(Math.max(lineHeight + verticalChrome, minHeight))
    const minRowsHeight = Math.ceil(Math.max(minRowCount * lineHeight + verticalChrome, minHeight))
    const cappedHeight = Math.ceil(maxRowCount * lineHeight + verticalChrome)
    const previousScrollTop = element.scrollTop
    const compactTextWidth = resolveCompactTextWidth(element, computed)
    if (!multilineRef.current || compactTextWidthRef.current <= 0) {
      compactTextWidthRef.current = compactTextWidth
    }
    const textWidthLimit = compactTextWidthRef.current || compactTextWidth
    const text = editor ? serializeComposerDoc(editor.state.doc) : value

    element.style.height = 'auto'
    const hasValue = text.length > 0
    const measuredHeight = hasValue ? element.scrollHeight : minRowsHeight
    const nextHeight = Math.max(minRowsHeight, Math.min(measuredHeight, cappedHeight))
    const overflows = hasValue && measuredHeight > cappedHeight
    const wrappedByHeight = measuredHeight > oneRowHeight
    const wrappedByWidth = textWidthLimit > 0 && measureLongestLineWidth(text, computed) > textWidthLimit
    const wrapped = hasValue && (wrappedByHeight || wrappedByWidth)

    element.style.height = `${nextHeight}px`
    element.style.overflowY = overflows ? 'auto' : 'hidden'
    if (overflows) element.scrollTop = previousScrollTop

    const nextMultiline = multilineMode === 'always'
      ? true
      : hasValue && (multilineMode === 'sticky-until-empty' && multilineRef.current ? true : wrapped)
    multilineRef.current = nextMultiline
    setMultiline((current) => current === nextMultiline ? current : nextMultiline)
  }, [editor, maxRows, minRows, value])

  function handleDefaultKeyDown(event: KeyboardEvent) {
    if (event.key === 'Backspace' && deleteAdjacentToken(editor, 'before')) return true
    if (event.key === 'Delete' && deleteAdjacentToken(editor, 'after')) return true
    return false
  }

  function emitTrigger(target: Editor) {
    const trigger = findActiveTrigger(target) || findTrailingTrigger(serializeComposerDoc(target.state.doc))
    activeTriggerRef.current = trigger
    onTriggerChangeRef.current?.(trigger ? { kind: trigger.kind, query: trigger.query } : null)
  }

  function syncEmptyAndTokens(target: Editor, notifyRemoved = true) {
    const text = serializeComposerDoc(target.state.doc)
    setEmpty(text.length === 0)
    const tokens = collectTokenEntries(target.state.doc)
    const nextKeys = new Set(tokens.map((token) => token.key))
    if (notifyRemoved) {
      for (const previousKey of previousTokenKeysRef.current) {
        if (nextKeys.has(previousKey)) continue
        const removed = parseTokenKey(previousKey)
        if (removed) onTokenRemoveRef.current?.(removed)
      }
    }
    previousTokenKeysRef.current = nextKeys
  }

  function replaceActiveTrigger(target: Editor, content: string | Array<Record<string, unknown>>) {
    const active = activeTriggerRef.current
    if (!active) {
      target.chain().focus().insertContent(content).run()
      return
    }
    const from = textOffsetToDocPosition(target.state.doc, active.from)
    const to = textOffsetToDocPosition(target.state.doc, active.to)
    target
      .chain()
      .focus()
      .deleteRange({ from, to })
      .insertContent(content)
      .run()
  }

  function replaceTextRange(target: Editor, fromOffset: number, toOffset: number, replacement: string) {
    const from = textOffsetToDocPosition(target.state.doc, fromOffset)
    const to = textOffsetToDocPosition(target.state.doc, toOffset)
    if (!replacement) {
      target.chain().focus().deleteRange({ from, to }).run()
      return
    }
    target
      .chain()
      .focus()
      .deleteRange({ from, to })
      .insertContent(replacement)
      .run()
  }

  function ensureTokens(target: Editor, tokens: ComposerTokenSpec[]) {
    const existing = new Set(collectTokenEntries(target.state.doc).map((token) => token.key))
    const missing = tokens
      .filter((token) => token.value.trim())
      .filter((token) => !existing.has(tokenKey(token)))
    if (missing.length === 0) return
    const content = missing.flatMap((token) => tokenToInsertContent(token))
    target.chain().focus().insertContent(content).run()
  }

  return (
    <EditorContent
      ref={rootRef}
      editor={editor}
      style={style}
      data-nova-multiline={multiline ? 'true' : undefined}
      data-empty={empty ? 'true' : undefined}
      data-disabled={disabled ? 'true' : undefined}
      data-placeholder={placeholder}
      className={className}
    />
  )
})

interface ComposerParseOptions {
  skills: string[]
  files: string[]
  lore: Array<{ id: string; label: string }>
  styleScenes: string[]
}

function serializeComposerDoc(doc: ProseMirrorNode): string {
  const blocks: string[] = []
  doc.forEach((block) => {
    let text = ''
    block.forEach((child) => {
      text += serializeInlineNode(child)
    })
    blocks.push(text)
  })
  return blocks.join('\n')
}

export function textToComposerJSON(text: string, options: ComposerParseOptions = { skills: [], files: [], lore: [], styleScenes: [] }) {
  const lines = text.split(/\r\n|\r|\n/)
  return {
    type: 'doc',
    content: lines.map((line) => ({
      type: 'paragraph',
      content: tokenizeLine(line, options),
    })),
  }
}

export function serializeComposerJSON(doc: { content?: Array<{ content?: Array<{ type?: string; text?: string; attrs?: Record<string, unknown> }> }> }): string {
  return (doc.content || [])
    .map((block) => (block.content || []).map((node) => {
      if (node.type === 'text') return node.text || ''
      if (node.type === 'hardBreak') return '\n'
      if (node.type === 'composerToken') {
        return tokenPlainText({
          kind: normalizeTokenKind(node.attrs?.kind),
          value: String(node.attrs?.value || ''),
          label: String(node.attrs?.label || node.attrs?.value || ''),
        })
      }
      return ''
    }).join(''))
    .join('\n')
}

function tokenizeLine(line: string, options: ComposerParseOptions) {
  const content: Array<Record<string, unknown>> = []
  let index = 0
  let pending = ''
  const sortedSkills = sortLongestFirst(options.skills)
  const sortedFiles = sortLongestFirst(options.files)
  const sortedLore = [...options.lore].sort((a, b) => b.label.length - a.label.length)
  const sortedStyles = sortLongestFirst(options.styleScenes)

  const flush = () => {
    if (!pending) return
    content.push({ type: 'text', text: pending })
    pending = ''
  }

  while (index < line.length) {
    const skill = matchKnownToken(line, index, '/', sortedSkills)
    if (skill) {
      flush()
      content.push(tokenNode({ kind: 'skill', value: skill, label: skill }))
      index += skill.length + 1
      continue
    }
    const file = matchKnownToken(line, index, '@', sortedFiles)
    if (file) {
      flush()
      content.push(tokenNode({ kind: 'file', value: file, label: file }))
      index += file.length + 1
      continue
    }
    const lore = matchKnownLore(line, index, sortedLore)
    if (lore) {
      flush()
      content.push(tokenNode({ kind: 'lore', value: lore.id, label: lore.label }))
      index += lore.label.length + '@资料:'.length
      continue
    }
    const style = matchKnownToken(line, index, '#', sortedStyles)
    if (style) {
      flush()
      content.push(tokenNode({ kind: 'style', value: style, label: style }))
      index += style.length + 1
      continue
    }
    pending += line[index]
    index += 1
  }
  flush()
  return content
}

function matchKnownToken(line: string, index: number, prefix: '/' | '@' | '#', values: string[]) {
  if (line[index] !== prefix) return ''
  for (const value of values) {
    if (!line.startsWith(`${prefix}${value}`, index)) continue
    const before = index === 0 ? '' : line[index - 1]
    const after = line[index + value.length + 1] || ''
    if (before && !/\s/.test(before)) continue
    if (after && !isTokenBoundary(after)) continue
    return value
  }
  return ''
}

function matchKnownLore(line: string, index: number, loreItems: Array<{ id: string; label: string }>) {
  const prefix = '@资料:'
  if (!line.startsWith(prefix, index)) return null
  for (const item of loreItems) {
    if (!line.startsWith(`${prefix}${item.label}`, index)) continue
    const before = index === 0 ? '' : line[index - 1]
    const after = line[index + prefix.length + item.label.length] || ''
    if (before && !/\s/.test(before)) continue
    if (after && !isTokenBoundary(after)) continue
    return item
  }
  return null
}

function isTokenBoundary(value: string) {
  return /\s|[,.;:!?，。！？、；：）)\]}]/.test(value)
}

function serializeInlineNode(node: ProseMirrorNode): string {
  if (node.type.name === 'text') return node.text || ''
  if (node.type.name === 'hardBreak') return '\n'
  if (node.type.name === 'composerToken') {
    return tokenPlainText({
      kind: normalizeTokenKind(node.attrs.kind),
      value: String(node.attrs.value || ''),
      label: String(node.attrs.label || node.attrs.value || ''),
    })
  }
  return node.textContent || ''
}

function tokenNode(token: ComposerTokenSpec) {
  return {
    type: 'composerToken',
    attrs: {
      kind: token.kind,
      value: token.value,
      label: token.label || token.value,
    },
  }
}

function tokenToInsertContent(token: ComposerTokenSpec) {
  return [
    tokenNode(token),
    { type: 'text', text: ' ' },
  ]
}

function tokenPlainText(token: ComposerTokenSpec) {
  const label = token.label || token.value
  if (token.kind === 'skill') return `/${token.value}`
  if (token.kind === 'style') return `#${token.value}`
  if (token.kind === 'lore') return `@资料:${label}`
  return `@${token.value}`
}

function tokenDisplayText(token: ComposerTokenSpec) {
  return tokenPlainText(token)
}

function normalizeTokenKind(kind: unknown): ComposerTokenKind {
  return kind === 'skill' || kind === 'lore' || kind === 'style' ? kind : 'file'
}

function collectTokenEntries(doc: ProseMirrorNode): TokenEntry[] {
  const tokens: TokenEntry[] = []
  doc.descendants((node) => {
    if (node.type.name !== 'composerToken') return true
    const token = {
      kind: normalizeTokenKind(node.attrs.kind),
      value: String(node.attrs.value || ''),
      label: String(node.attrs.label || node.attrs.value || ''),
    }
    tokens.push({ ...token, key: tokenKey(token) })
    return false
  })
  return tokens
}

function tokenKey(token: ComposerTokenSpec) {
  return `${token.kind}:${token.value}`
}

function parseTokenKey(key: string): ComposerTokenSpec | null {
  const index = key.indexOf(':')
  if (index <= 0) return null
  const kind = normalizeTokenKind(key.slice(0, index))
  const value = key.slice(index + 1)
  if (!value) return null
  return { kind, value }
}

function findActiveTrigger(editor: Editor): ActiveTriggerRange | null {
  const selection = editor.state.selection
  if (!selection.empty) return null
  const serialized = serializeComposerDoc(editor.state.doc)
  const cursor = docPositionToTextOffset(editor.state.doc, selection.from)
  const before = serialized.slice(0, cursor)
  const matches: Array<{ kind: ComposerTriggerKind; match: RegExpExecArray | null }> = [
    { kind: 'slash', match: /(^|\s)\/([^\s/]*)$/.exec(before) },
    { kind: 'reference', match: /(^|\s)@([^\s@]*)$/.exec(before) },
    { kind: 'style', match: /(^|\s)#([^\s#]*)$/.exec(before) },
  ]
  for (const item of matches) {
    const match = item.match
    if (!match) continue
    const prefixLength = match[1]?.length || 0
    const from = cursor - match[0].length + prefixLength
    return {
      kind: item.kind,
      query: match[2] || '',
      from,
      to: cursor,
    }
  }
  return null
}

function findTrailingTrigger(text: string): ActiveTriggerRange | null {
  const matches: Array<{ kind: ComposerTriggerKind; match: RegExpExecArray | null }> = [
    { kind: 'slash', match: /(^|\s)\/([^\s/]*)$/.exec(text) },
    { kind: 'reference', match: /(^|\s)@([^\s@]*)$/.exec(text) },
    { kind: 'style', match: /(^|\s)#([^\s#]*)$/.exec(text) },
  ]
  for (const item of matches) {
    const match = item.match
    if (!match) continue
    const prefixLength = match[1]?.length || 0
    const to = text.length
    const from = to - match[0].length + prefixLength
    return {
      kind: item.kind,
      query: match[2] || '',
      from,
      to,
    }
  }
  return null
}

function selectionTextOffsets(editor: Editor) {
  return {
    from: docPositionToTextOffset(editor.state.doc, editor.state.selection.from),
    to: docPositionToTextOffset(editor.state.doc, editor.state.selection.to),
  }
}

function docPositionToTextOffset(doc: ProseMirrorNode, position: number): number {
  let textOffset = 0
  let resolved = 0
  let found = false

  doc.forEach((block, blockOffset, blockIndex) => {
    if (found) return
    if (blockIndex > 0) textOffset += 1
    const blockStart = blockOffset + 1
    if (position <= blockStart) {
      resolved = textOffset
      found = true
      return
    }
    block.forEach((child, childOffset) => {
      if (found) return
      const childStart = blockStart + childOffset
      const childEnd = childStart + child.nodeSize
      const serialized = serializeInlineNode(child)
      if (position <= childStart) {
        resolved = textOffset
        found = true
        return
      }
      if (position < childEnd) {
        if (child.type.name === 'text') {
          resolved = textOffset + Math.max(0, position - childStart)
        } else {
          resolved = position <= childStart ? textOffset : textOffset + serialized.length
        }
        found = true
        return
      }
      textOffset += serialized.length
    })
    if (!found && position <= blockStart + block.content.size + 1) {
      resolved = textOffset
      found = true
    }
  })

  return found ? resolved : serializeComposerDoc(doc).length
}

function textOffsetToDocPosition(doc: ProseMirrorNode, targetOffset: number): number {
  const target = Math.max(0, targetOffset)
  let textOffset = 0
  let resolved = 1
  let found = false

  doc.forEach((block, blockOffset, blockIndex) => {
    if (found) return
    if (blockIndex > 0) {
      if (target <= textOffset) {
        resolved = blockOffset
        found = true
        return
      }
      textOffset += 1
    }
    const blockStart = blockOffset + 1
    resolved = blockStart
    block.forEach((child, childOffset) => {
      if (found) return
      const childStart = blockStart + childOffset
      const serialized = serializeInlineNode(child)
      const childEndOffset = textOffset + serialized.length
      if (target <= childEndOffset) {
        if (child.type.name === 'text') {
          resolved = childStart + Math.max(0, target - textOffset)
        } else {
          resolved = target <= textOffset ? childStart : childStart + child.nodeSize
        }
        found = true
        return
      }
      textOffset = childEndOffset
      resolved = childStart + child.nodeSize
    })
    if (!found && target <= textOffset) {
      found = true
    }
  })

  return Math.max(1, Math.min(resolved, doc.content.size))
}

function deleteAdjacentToken(editor: Editor | null, direction: 'before' | 'after') {
  if (!editor) return false
  const { selection } = editor.state
  if (!selection.empty) return false
  const $pos = selection.$from
  const node = direction === 'before' ? $pos.nodeBefore : $pos.nodeAfter
  if (!node || node.type.name !== 'composerToken') return false
  const from = direction === 'before' ? selection.from - node.nodeSize : selection.from
  const to = direction === 'before' ? selection.from : selection.from + node.nodeSize
  editor.commands.deleteRange({ from, to })
  return true
}

function parseCssPixels(value: string) {
  const parsed = Number.parseFloat(value)
  return Number.isFinite(parsed) ? parsed : 0
}

function resolveCompactTextWidth(element: HTMLElement, computed: CSSStyleDeclaration) {
  const paddingLeft = parseCssPixels(computed.paddingLeft)
  const paddingRight = parseCssPixels(computed.paddingRight)
  const composerWidth = resolveComposerCompactInputWidth(element)
  const fallbackWidth = element.clientWidth || parseCssPixels(computed.width)
  return Math.max(0, (composerWidth || fallbackWidth) - paddingLeft - paddingRight)
}

function resolveComposerCompactInputWidth(element: HTMLElement) {
  const toolbar = element.closest<HTMLElement>('.nova-agent-composer-toolbar')
  if (!toolbar) return 0

  const start = toolbar.querySelector<HTMLElement>('[data-slot="agent-composer-start"]')
  const end = toolbar.querySelector<HTMLElement>('[data-slot="agent-composer-end"]')
  const toolbarStyle = window.getComputedStyle(toolbar)
  const gap = parseCssPixels(toolbarStyle.columnGap || toolbarStyle.gap)
  const toolbarWidth = toolbar.getBoundingClientRect().width || toolbar.clientWidth
  const startWidth = start?.getBoundingClientRect().width || 0
  const endWidth = end?.getBoundingClientRect().width || 0
  return Math.max(0, toolbarWidth - startWidth - endWidth - gap * 2)
}

function measureLongestLineWidth(value: string, computed: CSSStyleDeclaration) {
  if (!value) return 0
  const canvas = textMeasureCanvas ?? (textMeasureCanvas = document.createElement('canvas'))
  const context = canvas.getContext('2d')
  if (!context) return 0

  context.font = computed.font || `${computed.fontStyle || 'normal'} ${computed.fontVariant || 'normal'} ${computed.fontWeight || '400'} ${computed.fontSize || '16px'} ${computed.fontFamily || 'sans-serif'}`
  return value
    .split(/\r\n|\r|\n/)
    .reduce((maxWidth, line) => Math.max(maxWidth, context.measureText(line).width), 0)
}

function sortLongestFirst(values: string[]) {
  return uniqueStrings(values).sort((a, b) => b.length - a.length)
}

function uniqueStrings(values: string[]) {
  return Array.from(new Set(values.map((value) => value.trim()).filter(Boolean)))
}
