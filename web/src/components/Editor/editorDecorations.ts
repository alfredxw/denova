import { Extension } from '@tiptap/core'
import type { Editor } from '@tiptap/react'
import type { Node as ProseMirrorNode } from '@tiptap/pm/model'
import { Plugin, PluginKey, TextSelection as PmTextSelection } from '@tiptap/pm/state'
import { Decoration, DecorationSet } from '@tiptap/pm/view'

import { findDialogueHighlightRanges } from '@/lib/dialogue-highlight'

export interface SearchState {
  query: string
  index: number
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
  const matches = findSearchMatchesInDoc(doc, searchState.query)
  if (matches.length === 0) return DecorationSet.empty

  const currentIndex = clampIndex(searchState.index, matches.length)
  const decorations = matches.map((match, index) =>
    Decoration.inline(match.from, match.to, {
      class: index === currentIndex ? 'nova-search-match nova-search-current' : 'nova-search-match',
    }),
  )
  return DecorationSet.create(doc, decorations)
}

export function findSearchMatches(editor: Editor, query: string): SearchMatch[] {
  return findSearchMatchesInDoc(editor.state.doc, query)
}

function findSearchMatchesInDoc(doc: ProseMirrorNode, query: string): SearchMatch[] {
  const normalizedQuery = query.trim().toLowerCase()
  if (!normalizedQuery) return []

  const matches: SearchMatch[] = []
  doc.descendants((node, pos) => {
    if (!node.isText || !node.text) return

    const normalizedText = node.text.toLowerCase()
    let searchFrom = 0
    while (searchFrom < normalizedText.length) {
      const index = normalizedText.indexOf(normalizedQuery, searchFrom)
      if (index === -1) break
      matches.push({
        from: pos + index,
        to: pos + index + normalizedQuery.length,
      })
      searchFrom = index + normalizedQuery.length
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

export function clampIndex(index: number, length: number) {
  return ((index % length) + length) % length
}
