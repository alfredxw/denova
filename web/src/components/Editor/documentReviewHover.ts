import type { Editor } from '@tiptap/react'
import { textBlockRangeAtPosition, type EditorReviewRange } from './documentReviewAnchors'

/** Maps any point in the editor surface to its nearest underlying text block. */
export function documentReviewRangeAtCoordinates(editor: Editor, clientX: number, clientY: number): EditorReviewRange | null {
  const editorRect = editor.view.dom.getBoundingClientRect()
  if (editorRect.width <= 2 || clientY < editorRect.top || clientY > editorRect.bottom) return null
  const editorStyle = getComputedStyle(editor.view.dom)
  const paddingLeft = Math.max(0, Number.parseFloat(editorStyle.paddingLeft) || 0)
  const paddingRight = Math.max(0, Number.parseFloat(editorStyle.paddingRight) || 0)
  // ProseMirror cannot resolve coordinates inside the editor's padding. The
  // line action lives in that gutter, so project its pointer onto the nearest
  // edge of the editable content box before asking ProseMirror for a position.
  const contentLeft = Math.min(editorRect.right - 1, editorRect.left + paddingLeft + 1)
  const contentRight = Math.max(contentLeft, editorRect.right - paddingRight - 1)
  const left = Math.max(contentLeft, Math.min(contentRight, clientX))
  const position = editor.view.posAtCoords({ left, top: clientY })?.pos
  return position === undefined ? null : textBlockRangeAtPosition(editor.state.doc, position)
}
