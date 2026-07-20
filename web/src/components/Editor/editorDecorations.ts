import { Extension } from '@tiptap/core'
import type { Editor } from '@tiptap/react'
import type { Node as ProseMirrorNode } from '@tiptap/pm/model'
import { Plugin, PluginKey, TextSelection as PmTextSelection } from '@tiptap/pm/state'
import { Decoration, DecorationSet } from '@tiptap/pm/view'

import { findDialogueHighlightRanges } from '@/lib/dialogue-highlight'

export interface SearchState {
  query: string
  index: number
  useRegex: boolean
}

export interface SearchMatch {
  from: number
  to: number
}

export const searchPluginKey = new PluginKey<DecorationSet>('nova-search-highlight')
const dialogueHighlightPluginKey = new PluginKey<DecorationSet>('nova-editor-dialogue-highlight')

/** 创建编辑器搜索高亮扩展，使用 ProseMirror Decoration 标记匹配项。 */
export function createSearchHighlightExtension(searchStateRef: { current: SearchState }) {
  return Extension.create({
    name: 'novaSearchHighlight',

    addProseMirrorPlugins() {
      return [
        new Plugin<DecorationSet>({
          key: searchPluginKey,
          state: {
            init: (_, state) => createSearchDecorations(state.doc, searchStateRef.current),
            apply: (tr, previousDecorations, _oldState, newState) => {
              if (tr.docChanged || tr.getMeta(searchPluginKey)) {
                return createSearchDecorations(newState.doc, searchStateRef.current)
              }
              return previousDecorations.map(tr.mapping, tr.doc)
            },
          },
          props: {
            decorations: (state) => searchPluginKey.getState(state) ?? DecorationSet.empty,
          },
        }),
      ]
    },
  })
}

/** 创建编辑器对白高亮扩展，不改变正文内容，仅用 Decoration 标记可视样式。 */
export function createDialogueHighlightExtension() {
  return Extension.create({
    name: 'novaEditorDialogueHighlight',

    addProseMirrorPlugins() {
      return [
        new Plugin<DecorationSet>({
          key: dialogueHighlightPluginKey,
          state: {
            init: (_, state) => createDialogueDecorations(state.doc),
            apply: (tr, previousDecorations, _oldState, newState) => {
              if (tr.docChanged) return createDialogueDecorations(newState.doc)
              return previousDecorations.map(tr.mapping, tr.doc)
            },
          },
          props: {
            decorations: (state) => dialogueHighlightPluginKey.getState(state) ?? DecorationSet.empty,
          },
        }),
      ]
    },
  })
}

function createDialogueDecorations(doc: ProseMirrorNode) {
  const decorations: Decoration[] = []
  doc.descendants((node, pos) => {
    if (!node.isText || !node.text) return
    for (const range of findDialogueHighlightRanges(node.text)) {
      decorations.push(Decoration.inline(pos + range.from, pos + range.to, { class: 'nova-editor-dialogue-highlight' }))
    }
  })
  return decorations.length > 0 ? DecorationSet.create(doc, decorations) : DecorationSet.empty
}

function createSearchDecorations(doc: ProseMirrorNode, searchState: SearchState) {
  const matches = findSearchMatchesInDoc(doc, searchState.query, searchState.useRegex)
  if (matches.length === 0) return DecorationSet.empty

  const currentIndex = clampIndex(searchState.index, matches.length)
  const decorations = matches.map((match, index) =>
    Decoration.inline(match.from, match.to, {
      class: index === currentIndex ? 'nova-search-match nova-search-current' : 'nova-search-match',
    }),
  )
  return DecorationSet.create(doc, decorations)
}

export function findSearchMatches(editor: Editor, query: string, useRegex = false): SearchMatch[] {
  return findSearchMatchesInDoc(editor.state.doc, query, useRegex)
}

function findSearchMatchesInDoc(doc: ProseMirrorNode, query: string, useRegex: boolean): SearchMatch[] {
  const trimmedQuery = query.trim()
  if (!trimmedQuery) return []

  let regex: RegExp | null = null
  if (useRegex) {
    try {
      regex = new RegExp(trimmedQuery, 'gi')
    } catch {
      return []
    }
  }

  const matches: SearchMatch[] = []
  doc.descendants((node, pos) => {
    if (!node.isText || !node.text) return

    if (regex) {
      regex.lastIndex = 0
      let match: RegExpExecArray | null
      while ((match = regex.exec(node.text)) !== null) {
        if (match[0].length === 0) {
          regex.lastIndex++
          continue
        }
        matches.push({
          from: pos + match.index,
          to: pos + match.index + match[0].length,
        })
      }
    } else {
      const lowerText = node.text.toLowerCase()
      const lowerQuery = trimmedQuery.toLowerCase()
      let searchFrom = 0
      while (searchFrom < lowerText.length) {
        const index = lowerText.indexOf(lowerQuery, searchFrom)
        if (index === -1) break
        matches.push({
          from: pos + index,
          to: pos + index + lowerQuery.length,
        })
        searchFrom = index + lowerQuery.length
      }
    }
  })
  return matches
}

export function selectSearchMatch(editor: Editor, match: SearchMatch) {
  const selection = PmTextSelection.create(editor.state.doc, match.from, match.to)
  editor.view.dispatch(editor.state.tr.setSelection(selection).scrollIntoView())
  // 额外使用 DOM scrollIntoView 确保 scroll-margin-top 生效（避免被 sticky 搜索栏遮挡）
  requestAnimationFrame(() => {
    const el = editor.view.dom.querySelector('.nova-search-current') as HTMLElement | null
    el?.scrollIntoView({ block: 'nearest', behavior: 'smooth' })
  })
}

/** 编译搜索正则；无效正则返回 null。 */
function compileSearchRegex(query: string): RegExp | null {
  const trimmed = query.trim()
  if (!trimmed) return null
  try {
    return new RegExp(trimmed, 'gi')
  } catch {
    return null
  }
}

/** 根据匹配文本和替换模板生成最终替换文本（正则模式下支持 $1 等捕获组引用）。 */
function buildReplacementText(matchedText: string, query: string, replacement: string, useRegex: boolean): string {
  if (!useRegex) return replacement
  const regex = compileSearchRegex(query)
  if (!regex) return replacement
  regex.lastIndex = 0
  return matchedText.replace(regex, replacement)
}

/** 替换当前选中的匹配项，返回是否执行了替换。 */
export function replaceCurrentSearchMatch(
  editor: Editor,
  query: string,
  replacement: string,
  useRegex: boolean,
  currentIndex: number,
): boolean {
  const matches = findSearchMatches(editor, query, useRegex)
  if (matches.length === 0) return false

  const index = clampIndex(currentIndex, matches.length)
  const match = matches[index]
  const matchedText = editor.state.doc.textBetween(match.from, match.to)
  const newText = buildReplacementText(matchedText, query, replacement, useRegex)

  const tr = editor.state.tr
  tr.replaceRangeWith(match.from, match.to, editor.state.schema.text(newText))
  editor.view.dispatch(tr)
  return true
}

/** 批量替换所有匹配项，返回替换数量。从后向前替换以避免位置偏移。 */
export function replaceAllSearchMatches(
  editor: Editor,
  query: string,
  replacement: string,
  useRegex: boolean,
): number {
  const matches = findSearchMatches(editor, query, useRegex)
  if (matches.length === 0) return 0

  const tr = editor.state.tr
  for (let i = matches.length - 1; i >= 0; i--) {
    const match = matches[i]
    const matchedText = editor.state.doc.textBetween(match.from, match.to)
    const newText = buildReplacementText(matchedText, query, replacement, useRegex)
    tr.replaceRangeWith(match.from, match.to, editor.state.schema.text(newText))
  }
  editor.view.dispatch(tr)
  return matches.length
}

export function clampIndex(index: number, length: number) {
  return ((index % length) + length) % length
}
