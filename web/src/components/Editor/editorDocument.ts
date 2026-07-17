import { Node, mergeAttributes } from '@tiptap/core'
import Image from '@tiptap/extension-image'
import type { Node as ProseMirrorNode } from '@tiptap/pm/model'
import { EditorState } from '@tiptap/pm/state'
import type { Editor } from '@tiptap/react'

import { workspaceAssetURL } from '@/lib/api'

/** 检测文本是否已自带缩进（首个非空行以全角/半角空格开头）。 */
export function hasNativeIndent(text: string): boolean {
  const lines = text.split('\n')
  for (const line of lines) {
    if (!line.trim()) continue
    return /^[\s\u3000]{2,}/.test(line)
  }
  return false
}

/** 判断文件是否为纯文本（.txt）格式。 */
export function isTxtFile(name: string | null): boolean {
  return !!name && name.toLowerCase().endsWith('.txt')
}

export function isMarkdownFile(name: string | null): boolean {
  return !!name && /\.(md|markdown)$/i.test(name)
}

export function createWorkspaceImageExtension() {
  return Image.extend({
    renderHTML({ HTMLAttributes }) {
      const src = resolveWorkspaceImageSrc(HTMLAttributes.src)
      return ['img', mergeAttributes(this.options.HTMLAttributes, HTMLAttributes, { src })]
    },
  }).configure({
    inline: false,
    allowBase64: true,
  })
}

/** 与 StarterKit 中禁用的 hardBreak 对应，保留创作模式的段首缩进渲染。 */
export function createIndentedHardBreakExtension() {
  return Node.create({
    name: 'hardBreak',
    inline: true,
    group: 'inline',
    selectable: false,
    linebreakReplacement: true,
    parseHTML() {
      return [{ tag: 'br' }]
    },
    renderHTML() {
      return ['span', { class: 'nova-hard-break' }, ['br']]
    },
    addKeyboardShortcuts() {
      return {
        'Shift-Enter': () => {
          if (!this.editor || this.editor.isDestroyed) return false
          return this.editor.commands.setHardBreak()
        },
      }
    },
    addCommands() {
      return {
        setHardBreak: () => ({ commands }) => {
          return commands.first([
            () => commands.exitCode(),
            () => commands.insertContent({ type: this.name }),
          ])
        },
      }
    },
  })
}

function resolveWorkspaceImageSrc(src: unknown) {
  if (typeof src !== 'string' || src.trim() === '') return src
  const value = src.trim()
  if (/^(https?:|data:|blob:|\/)/i.test(value)) return value
  if (value.startsWith('assets/')) return workspaceAssetURL(value)
  return value
}

export function readEditorText(editor: Editor, fileName: string | null): string {
  return isTxtFile(fileName)
    ? normalizeEditorText(editor.getText({ blockSeparator: '\n' }))
    : normalizeEditorText(editor.getMarkdown())
}

/** 文件切换后重建编辑器状态，避免上一个文件的 Ctrl-Z 历史跨文件生效。 */
export function resetEditorStateHistory(editor: Editor) {
  const state = editor.state
  editor.view.updateState(EditorState.create({
    schema: state.schema,
    doc: state.doc,
    selection: state.selection,
    plugins: state.plugins,
  }))
}

export function normalizeEditorText(text: string): string {
  return text
    .replace(/\r\n/g, '\n')
    .split('\n')
    .map((line) => line.trimEnd())
    .join('\n')
    .replace(/\n{4,}/g, '\n\n\n')
    .trimEnd()
    .concat('\n')
}

export function updateCharacterStats(editor: Editor, setSelected: (value: number) => void) {
  const { from, to, empty } = editor.state.selection
  if (empty) {
    setSelected(0)
    return
  }
  setSelected(countTextCharacters(editor.state.doc.textBetween(from, to, '\n')))
}

export function countTextCharacters(text: string) {
  return Array.from(text.replace(/\s/g, '')).length
}

/** 计算文档中某位置对应的行号（从 1 开始）。 */
export function getLineNumber(doc: ProseMirrorNode, pos: number): number {
  let line = 1
  doc.forEach((node, nodeOffset) => {
    if (nodeOffset + node.nodeSize <= pos) {
      line++
    }
  })
  return line
}
