import { createFileTreeIconResolver, getBuiltInSpriteSheet } from '@pierre/trees'
import type { DiffFileKind } from './types'

const fileIconResolver = createFileTreeIconResolver({ set: 'complete', colored: true })
const fileIconSprite = getBuiltInSpriteSheet('complete')

/** Keeps CodeView file headers visually aligned with Pierre's file tree. */
export function DiffFileIcon({ path, kind }: { path: string; kind: DiffFileKind }) {
  const icon = fileIconResolver.resolveIcon('file-tree-icon-file', path)
  return (
    <svg
      aria-hidden="true"
      data-diff-file-kind={kind}
      data-icon-token={icon.token}
      viewBox={icon.viewBox ?? '0 0 16 16'}
      className="nova-code-diff-file-icon size-3.5 shrink-0"
    >
      <use href={`#${icon.name}`} />
    </svg>
  )
}

/** The dependency owns this static SVG sprite; no user-provided HTML enters it. */
export function DiffFileIconSprite() {
  return <span aria-hidden="true" className="absolute size-0 overflow-hidden" dangerouslySetInnerHTML={{ __html: fileIconSprite }} />
}
