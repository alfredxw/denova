import { useMemo, type MouseEvent } from 'react'
import { ThemedMarkdownRenderer, type MarkdownRendererComponents } from '@/components/common/MarkdownRenderer'
import { projectFileAssetURL } from './api'

interface ProjectMarkdownPreviewProps {
  projectId: string
  path: string
  content: string
  onOpenFile: (path: string) => void
}

/** Safe Markdown preview with project-relative images and file navigation. */
export function ProjectMarkdownPreview({ projectId, path, content, onOpenFile }: ProjectMarkdownPreviewProps) {
  const components = useMemo<MarkdownRendererComponents>(() => ({
    a: ({ href, children, node: _node, ...props }) => {
      const projectPath = resolveMarkdownProjectPath(path, href)
      if (!projectPath) {
        const external = Boolean(href && /^(?:https?:|mailto:)/i.test(href))
        if (external || href?.startsWith('#')) {
          return <a {...props} href={href} target={external ? '_blank' : undefined} rel={external ? 'noreferrer' : undefined}>{children}</a>
        }
        return <span {...props}>{children}</span>
      }
      const open = (event: MouseEvent<HTMLAnchorElement>) => {
        event.preventDefault()
        onOpenFile(projectPath)
      }
      return <a {...props} href="#" title={projectPath} onClick={open}>{children}</a>
    },
    img: ({ src, alt, node: _node, ...props }) => {
      const projectPath = resolveMarkdownProjectPath(path, src)
      const external = Boolean(src && /^https?:/i.test(src))
      return (
        <img
          {...props}
          src={projectPath ? projectFileAssetURL(projectId, projectPath) : external ? src : undefined}
          alt={alt ?? ''}
          loading="lazy"
          className="max-h-[70vh] max-w-full rounded-lg border border-[var(--nova-border)] object-contain"
        />
      )
    },
  }), [onOpenFile, path, projectId])

  return (
    <div className="min-h-0 flex-1 overflow-y-auto bg-[var(--nova-bg)] px-5 py-5 sm:px-7">
      <ThemedMarkdownRenderer content={content} components={components} className="mx-auto max-w-4xl text-sm leading-6" />
    </div>
  )
}

export function resolveMarkdownProjectPath(documentPath: string, target: string | undefined): string | null {
  const trimmed = target?.trim() ?? ''
  if (!trimmed || trimmed.startsWith('#') || trimmed.startsWith('//') || /^[a-z][a-z\d+.-]*:/i.test(trimmed)) return null
  let pathname = trimmed.split(/[?#]/, 1)[0]
  try {
    pathname = decodeURIComponent(pathname)
  } catch {
    return null
  }
  if (!pathname || pathname.includes('\u0000')) return null
  const components = pathname.startsWith('/')
    ? []
    : documentPath.split('/').slice(0, -1).filter(Boolean)
  for (const component of pathname.split('/')) {
    if (!component || component === '.') continue
    if (component === '..') {
      if (components.length === 0) return null
      components.pop()
      continue
    }
    components.push(component)
  }
  return components.length > 0 ? components.join('/') : null
}
