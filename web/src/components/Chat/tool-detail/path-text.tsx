import { Fragment } from 'react'
import { useToolNavigation } from '../tool-navigation'
import { cn } from '@/lib/utils'

const LINK_CANDIDATE = /(["'][^"'\n]+["']|(?:[A-Za-z]:[\\/]|\/|\.\.?[\\/])?[A-Za-z0-9_@~.+-]+(?:[\\/][^\s"'`|<>]+)+(?::\d+(?::\d+)?)?|(?:\.\.?[\\/])?[A-Za-z0-9_@~-]+\.[A-Za-z0-9_-]+(?::\d+(?::\d+)?)?)/g
const KNOWN_EXTENSIONLESS_FILES = new Set(['Dockerfile', 'Makefile', 'LICENSE', 'README'])

interface WorkspacePathTextProps {
  children: string
  cwd?: string
  className?: string
}

export function WorkspacePathText({ children, cwd, className }: WorkspacePathTextProps) {
  const navigation = useToolNavigation()
  const path = navigation ? resolveWorkspaceFilePath(children, navigation.workspace, cwd) : null
  if (!path || !navigation) return className ? <span className={className}>{children}</span> : <>{children}</>
  return (
    <button
      type="button"
      data-nova-workspace-path={path}
      className={cn('cursor-pointer text-left font-mono text-inherit hover:text-[var(--nova-text)] hover:underline focus-visible:text-[var(--nova-text)] focus-visible:underline focus-visible:outline-none', className)}
      onClick={(event) => {
        event.stopPropagation()
        navigation.open({ kind: 'workspace_file', path })
      }}
    >
      {children}
    </button>
  )
}

export function LinkifyWorkspacePaths({ text, cwd }: { text: string; cwd?: string }) {
  const matches = [...text.matchAll(LINK_CANDIDATE)]
  if (matches.length === 0) return <>{text}</>
  let cursor = 0
  return (
    <>
      {matches.map((match, index) => {
        const start = match.index || 0
        const before = text.slice(cursor, start)
        cursor = start + match[0].length
        return (
          <Fragment key={`${start}-${index}`}>
            {before}
            <WorkspacePathText cwd={cwd}>{match[0]}</WorkspacePathText>
          </Fragment>
        )
      })}
      {text.slice(cursor)}
    </>
  )
}

export function resolveWorkspaceFilePath(candidate: string, workspace: string, cwd = ''): string | null {
  const cleaned = cleanCandidate(candidate)
  if (!cleaned || hasGlob(cleaned) || isURI(cleaned) || cleaned.endsWith('/') || cleaned.endsWith('\\')) return null

  const withoutPosition = stripLinePosition(cleaned)
  if (!looksLikeFile(withoutPosition)) return null

  const normalizedWorkspace = normalizeAbsolute(workspace)
  const normalizedCandidate = withoutPosition.replace(/\\/g, '/')
  if (isAbsolutePath(normalizedCandidate)) {
    return normalizedWorkspace ? relativeToWorkspace(normalizedCandidate, normalizedWorkspace) : null
  }

  let base = ''
  if (cwd.trim()) {
    const normalizedCwd = cwd.trim().replace(/\\/g, '/')
    if (isAbsolutePath(normalizedCwd)) {
      if (!normalizedWorkspace) return null
      const relativeCwd = relativeToWorkspace(normalizedCwd, normalizedWorkspace)
      if (relativeCwd === null) return null
      base = relativeCwd
    } else {
      base = normalizedCwd
    }
  }
  return normalizeRelative([base, normalizedCandidate].filter(Boolean).join('/'))
}

function cleanCandidate(value: string) {
  let result = value.trim()
  if ((result.startsWith('"') && result.endsWith('"')) || (result.startsWith("'") && result.endsWith("'"))) {
    result = result.slice(1, -1).trim()
  }
  return result.replace(/[),;]+$/, '')
}

function stripLinePosition(value: string) {
  const match = value.match(/^(.*?):\d+(?::\d+)?$/)
  if (!match) return value
  if (/^[A-Za-z]$/.test(match[1])) return value
  return match[1]
}

function hasGlob(value: string) {
  return /[*?\[\]{}]/.test(value)
}

function isURI(value: string) {
  return /^[A-Za-z][A-Za-z0-9+.-]*:\/\//.test(value) || (/^[A-Za-z][A-Za-z0-9+.-]*:/.test(value) && !/^[A-Za-z]:[\\/]/.test(value))
}

function looksLikeFile(value: string) {
  const basename = value.replace(/\\/g, '/').split('/').pop() || ''
  return basename.startsWith('.') || basename.includes('.') || KNOWN_EXTENSIONLESS_FILES.has(basename)
}

function isAbsolutePath(value: string) {
  return value.startsWith('/') || /^[A-Za-z]:\//.test(value)
}

function normalizeAbsolute(value: string) {
  const normalized = value.trim().replace(/\\/g, '/').replace(/\/+$/, '')
  if (!isAbsolutePath(normalized)) return ''
  const prefix = normalized.startsWith('/') ? '/' : normalized.slice(0, 3)
  const rest = normalized.startsWith('/') ? normalized.slice(1) : normalized.slice(3)
  const relative = normalizeRelative(rest)
  return relative === null ? '' : `${prefix}${relative}`.replace(/\/$/, '')
}

function relativeToWorkspace(value: string, workspace: string) {
  const normalizedValue = normalizeAbsolute(value)
  if (!normalizedValue) return null
  const windows = /^[A-Za-z]:\//.test(workspace)
  const comparableValue = windows ? normalizedValue.toLowerCase() : normalizedValue
  const comparableWorkspace = windows ? workspace.toLowerCase() : workspace
  if (comparableValue === comparableWorkspace) return null
  if (!comparableValue.startsWith(`${comparableWorkspace}/`)) return null
  return normalizeRelative(normalizedValue.slice(workspace.length + 1))
}

function normalizeRelative(value: string): string | null {
  const segments: string[] = []
  for (const segment of value.replace(/\\/g, '/').split('/')) {
    if (!segment || segment === '.') continue
    if (segment === '..') {
      if (segments.length === 0) return null
      segments.pop()
      continue
    }
    segments.push(segment)
  }
  return segments.length > 0 ? segments.join('/') : null
}
