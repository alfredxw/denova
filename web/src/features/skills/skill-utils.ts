import { AGENTS } from '@/features/agents/agent-registry'
import type { AgentViewDefinition, VisibleAgentKey } from '@/features/agents/agent-registry'
import type { FileNode } from '@/hooks/useWorkspace'
import type { SkillDocument, SkillFile, SkillInstallCandidate, SkillScope, SkillScopeInfo, SkillSummary } from '@/lib/api'

export type SkillsMode = 'editor' | 'create' | 'config' | 'install'
export type SkillInstallSource = 'remote' | 'zip'
export type SkillContentViewMode = 'preview' | 'raw'

export const skillNamePattern = /^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$/
export const skillEntryFile = 'SKILL.md'
export const skillScopes: SkillScope[] = ['user', 'workspace', 'builtin']
export const skillAgentOptions = AGENTS.filter((agent) => agent.capabilityMode === 'tools')

export function keyOf(skill: Pick<SkillSummary, 'scope' | 'name'>) {
  return `${skill.scope}:${skill.name}`
}

export function skillFilePath(scope: SkillScopeInfo | undefined, name: string) {
  if (!scope?.path) return ''
  return `${scope.path.replace(/[\\/]+$/, '')}/${name}/SKILL.md`
}

export function skillDisplayPath(document: SkillDocument, filePath: string) {
  if (filePath === skillEntryFile) return document.path
  const root = document.path.replace(/[\\/]SKILL\.md$/, '')
  return `${root}/${filePath}`
}

export function skillFilesForDocument(document: SkillDocument): SkillFile[] {
  const files = document.files || []
  if (files.some((file) => file.path === skillEntryFile)) return files
  return [
    {
      path: skillEntryFile,
      size: new Blob([document.content]).size,
      entry: true,
      editable: document.editable,
      updated_at: document.updated_at,
    },
    ...files,
  ]
}

export function skillFileTreeForDocument(document: SkillDocument): FileNode[] {
  const roots: FileNode[] = []
  for (const file of skillFilesForDocument(document)) {
    appendSkillFileTreeNode(roots, file.path.split('/').filter(Boolean))
  }
  return roots
}

function appendSkillFileTreeNode(nodes: FileNode[], parts: string[]) {
  const [name, ...rest] = parts
  if (!name) return
  if (rest.length === 0) {
    if (!nodes.some((node) => node.name === name && node.type === 'file')) {
      nodes.push({ name, type: 'file' })
    }
    return
  }
  let dir = nodes.find((node) => node.name === name && node.type === 'dir')
  if (!dir) {
    dir = { name, type: 'dir', children: [] }
    nodes.push(dir)
  }
  appendSkillFileTreeNode(dir.children ?? (dir.children = []), rest)
}

export function collectSkillFileTreeDirs(nodes: FileNode[], basePath = ''): string[] {
  const paths: string[] = []
  for (const node of nodes) {
    if (node.type !== 'dir') continue
    const path = basePath ? `${basePath}/${node.name}` : node.name
    paths.push(path)
    paths.push(...collectSkillFileTreeDirs(node.children || [], path))
  }
  return paths
}

export function isMarkdownSkillFile(path: string) {
  return /\.(?:md|markdown)$/i.test(path)
}

export function stripSkillMarkdownFrontmatter(content: string) {
  return content.replace(/^\uFEFF?---[ \t]*\r?\n[\s\S]*?\r?\n---[ \t]*(?:\r?\n|$)/, '')
}

export function parseAgentKeys(agentField?: string): VisibleAgentKey[] {
  const allowed = new Set<string>(skillAgentOptions.map((agent) => agent.key))
  const seen = new Set<VisibleAgentKey>()
  const out: VisibleAgentKey[] = []
  for (const part of (agentField || '').split(/[,;\s]+/)) {
    if (!allowed.has(part)) continue
    const agent = part as VisibleAgentKey
    if (seen.has(agent)) continue
    seen.add(agent)
    out.push(agent)
  }
  return out
}

export function isInstallableCandidate(candidate: SkillInstallCandidate) {
  return !candidate.conflict && !candidate.invalid_reason
}

export function requireInstallFile(file: File | null, t: (key: string) => string): File {
  if (!file) throw new Error(t('skills.install.zipRequired'))
  return file
}

export function updateSkillConfigContent(content: string, name: string, description: string, agents: VisibleAgentKey[]) {
  const match = content.match(/^---\r?\n([\s\S]*?)\r?\n---(\r?\n?[\s\S]*)$/)
  if (!match) return content
  const newline = content.includes('\r\n') ? '\r\n' : '\n'
  const seen = { name: false, description: false, agent: false }
  const nextLines: string[] = []
  for (const line of match[1].split(/\r?\n/)) {
    const key = line.match(/^\s*([A-Za-z_][A-Za-z0-9_-]*)\s*:/)?.[1]
    if (key === 'name') {
      seen.name = true
      nextLines.push(`name: ${yamlString(name)}`)
      continue
    }
    if (key === 'description') {
      seen.description = true
      nextLines.push(`description: ${yamlString(description)}`)
      continue
    }
    if (key === 'agent') {
      seen.agent = true
      if (agents.length > 0) nextLines.push(`agent: ${yamlString(agents.join(','))}`)
      continue
    }
    nextLines.push(line)
  }
  if (!seen.name) nextLines.unshift(`name: ${yamlString(name)}`)
  if (!seen.description) nextLines.push(`description: ${yamlString(description)}`)
  if (!seen.agent && agents.length > 0) nextLines.push(`agent: ${yamlString(agents.join(','))}`)
  return `---${newline}${nextLines.join(newline)}${newline}---${match[2]}`
}

function yamlString(value: string) {
  return JSON.stringify(value)
}

export function scopeLabel(scope: SkillScope, t: (key: string) => string) {
  if (scope === 'workspace') return t('skills.scope.workspace')
  if (scope === 'user') return t('skills.scope.user')
  return t('skills.scope.builtin')
}

export function preferredBuiltinOverrideScope(scopes: SkillScopeInfo[]) {
  return (scopes.find((scope) => scope.scope === 'user' && scope.writable) ||
    scopes.find((scope) => scope.scope === 'workspace' && scope.writable)) ?? null
}

export function groupSkillAgents(agentOptions: AgentViewDefinition[]) {
  return agentOptions.reduce<Array<{ group: string; agents: AgentViewDefinition[] }>>((groups, agent) => {
    const last = groups[groups.length - 1]
    if (last?.group === agent.groupKey) {
      last.agents.push(agent)
    } else {
      groups.push({ group: agent.groupKey, agents: [agent] })
    }
    return groups
  }, [])
}
