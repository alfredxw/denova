import type { Monaco } from '@monaco-editor/react'

/** Releases retained DiffEditor models only after Monaco has detached the widget. */
export function scheduleDetachedReviewModelDisposal(monaco: Monaco, modelPaths: readonly string[]) {
  queueMicrotask(() => {
    for (const path of modelPaths) {
      const model = monaco.editor.getModel(monaco.Uri.parse(path))
      if (model && !model.isDisposed() && !model.isAttachedToEditor()) model.dispose()
    }
  })
}
