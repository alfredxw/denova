import type { Editor } from '@tiptap/react'
import { textBlockRangeAtPosition, type EditorReviewRange } from './documentReviewAnchors'

/** Maps any point in the editor surface to its nearest underlying text block. */
export function documentReviewRangeAtCoordinates(editor: Editor, clientX: number, clientY: number): EditorReviewRange | null {
  const editorRect = editor.view.dom.getBoundingClientRect()
  if (editorRect.width <= 2 || clientY < editorRect.top || clientY > editorRect.bottom) return null
  const left = Math.max(editorRect.left + 1, Math.min(editorRect.right - 1, clientX))
  const position = editor.view.posAtCoords({ left, top: clientY })?.pos
  return position === undefined ? null : textBlockRangeAtPosition(editor.state.doc, position)
}
