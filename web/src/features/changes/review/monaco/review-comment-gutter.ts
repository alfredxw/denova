import type { Monaco } from '@monaco-editor/react'
import type { IDisposable, editor } from 'monaco-editor'

interface ReviewCommentGutterOptions {
  id: string
  labelForLine: (lineNumber: number) => string
  isCommentable: (lineNumber: number) => boolean
  onRequest: (lineNumber: number) => void
}

/** Keeps one virtualized comment button on the line currently under the pointer. */
export class ReviewCommentGutter implements IDisposable {
  private readonly editor: editor.IStandaloneCodeEditor
  private readonly monaco: Monaco
  private readonly options: ReviewCommentGutterOptions
  private readonly disposables: IDisposable[] = []
  private readonly domNode: HTMLButtonElement
  private readonly widget: editor.IGlyphMarginWidget
  private lineNumber = 1
  private disabled = false
  private disposed = false

  constructor(editorInstance: editor.IStandaloneCodeEditor, monaco: Monaco, options: ReviewCommentGutterOptions) {
    this.editor = editorInstance
    this.monaco = monaco
    this.options = options
    this.domNode = document.createElement('button')
    this.domNode.type = 'button'
    this.domNode.tabIndex = -1
    this.domNode.className = 'nova-review-glyph-button'
    this.domNode.dataset.visible = 'false'
    this.domNode.setAttribute('aria-hidden', 'true')
    const icon = document.createElement('span')
    icon.className = 'codicon codicon-add'
    icon.setAttribute('aria-hidden', 'true')
    this.domNode.append(icon)
    this.widget = {
      getId: () => `denova.change-review.hover-comment.${options.id}`,
      getDomNode: () => this.domNode,
      getPosition: () => ({
        lane: this.monaco.editor.GlyphMarginLane.Center,
        range: new this.monaco.Range(this.lineNumber, 1, this.lineNumber, 1),
        zIndex: 60,
      }),
    }
    this.domNode.addEventListener('click', this.handleClick)
    this.editor.addGlyphMarginWidget(this.widget)
    this.disposables.push(
      this.editor.onMouseMove((event) => this.handleMouseMove(event)),
      this.editor.onMouseLeave(() => this.hide()),
    )
  }

  updateDisabled(disabled: boolean) {
    this.disabled = disabled
    this.domNode.disabled = disabled
    this.domNode.setAttribute('aria-disabled', String(disabled))
    if (disabled) this.hide()
  }

  dispose() {
    if (this.disposed) return
    this.disposed = true
    this.domNode.removeEventListener('click', this.handleClick)
    this.editor.removeGlyphMarginWidget(this.widget)
    for (const disposable of this.disposables.splice(0)) disposable.dispose()
  }

  private readonly handleClick = (event: MouseEvent) => {
    event.preventDefault()
    event.stopPropagation()
    if (this.disabled || !this.options.isCommentable(this.lineNumber)) return
    this.options.onRequest(this.lineNumber)
  }

  private handleMouseMove(event: editor.IEditorMouseEvent) {
    const target = event.target
    if (!target.position
      || target.type === this.monaco.editor.MouseTargetType.CONTENT_VIEW_ZONE
      || target.type === this.monaco.editor.MouseTargetType.GUTTER_VIEW_ZONE) {
      this.hide()
      return
    }
    const lineNumber = target.position.lineNumber
    if (this.disabled || !this.options.isCommentable(lineNumber)) {
      this.hide()
      return
    }
    this.lineNumber = lineNumber
    const label = this.options.labelForLine(lineNumber)
    this.domNode.setAttribute('aria-label', label)
    this.domNode.title = label
    this.domNode.dataset.visible = 'true'
    this.domNode.setAttribute('aria-hidden', 'false')
    this.domNode.tabIndex = 0
    this.editor.layoutGlyphMarginWidget(this.widget)
  }

  private hide() {
    this.domNode.dataset.visible = 'false'
    this.domNode.setAttribute('aria-hidden', 'true')
    this.domNode.tabIndex = -1
  }
}
