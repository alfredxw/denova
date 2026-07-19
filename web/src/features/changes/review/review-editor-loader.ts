type ReviewDiffEditorModule = typeof import('./ReviewDiffEditor')

let reviewDiffEditorModule: Promise<ReviewDiffEditorModule> | null = null

export function loadReviewDiffEditor(): Promise<ReviewDiffEditorModule> {
  reviewDiffEditorModule ??= import('./ReviewDiffEditor')
  return reviewDiffEditorModule
}

/** Loads both the review component chunk and Monaco before the surface opens. */
export async function preloadReviewDiffEditor(): Promise<void> {
  const module = await loadReviewDiffEditor()
  await module.preloadReviewMonaco()
}
