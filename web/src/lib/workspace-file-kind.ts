export type WorkspaceFileKind = 'markdown' | 'image' | 'json' | 'jsonl' | 'other'

const IMAGE_EXTENSIONS = new Set(['.jpg', '.jpeg', '.png', '.webp', '.gif'])

export function workspaceFileKind(path?: string | null): WorkspaceFileKind {
  const ext = fileExtension(path)
  if (ext === '.md' || ext === '.markdown') return 'markdown'
  if (IMAGE_EXTENSIONS.has(ext)) return 'image'
  if (ext === '.json') return 'json'
  if (ext === '.jsonl') return 'jsonl'
  return 'other'
}

export function isWorkspaceImagePath(path?: string | null): boolean {
  return workspaceFileKind(path) === 'image'
}

export function isWorkspaceMarkdownPath(path?: string | null): boolean {
  return workspaceFileKind(path) === 'markdown'
}

function fileExtension(path?: string | null): string {
  const name = (path || '').trim().split(/[?#]/, 1)[0]
  const slashIndex = Math.max(name.lastIndexOf('/'), name.lastIndexOf('\\'))
  const base = slashIndex >= 0 ? name.slice(slashIndex + 1) : name
  const dotIndex = base.lastIndexOf('.')
  return dotIndex >= 0 ? base.slice(dotIndex).toLowerCase() : ''
}
