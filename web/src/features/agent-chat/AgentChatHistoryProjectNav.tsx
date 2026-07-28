import { Folder, PanelLeftClose } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import type { AgentChatProject } from './api'

interface HistoryProjectNavProps {
  projects: readonly AgentChatProject[]
  selectedProjectPath: string
  onSelect: (projectPath: string) => void
}

/** Desktop master pane for choosing the project whose durable conversations are shown. */
export function AgentChatHistoryProjectSidebar({
  projects,
  selectedProjectPath,
  currentProjectPath,
  onSelect,
  onCollapse,
}: HistoryProjectNavProps & {
  currentProjectPath: string
  onCollapse: () => void
}) {
  const { t } = useTranslation()
  return (
    <aside className="hidden w-44 shrink-0 flex-col border-r border-[var(--nova-border-soft)] bg-[var(--nova-surface-2)] sm:flex">
      <div className="flex h-10 shrink-0 items-center border-b border-[var(--nova-border-soft)] px-2">
        <span className="min-w-0 flex-1 truncate text-[10px] font-medium text-[var(--nova-text-muted)]">
          {t('agentChat.history.projects')}
        </span>
        <Button
          type="button"
          variant="ghost"
          size="icon-xs"
          className="shrink-0"
          onClick={onCollapse}
          aria-label={t('agentChat.history.hideProjects')}
          title={t('agentChat.history.hideProjects')}
        >
          <PanelLeftClose />
        </Button>
      </div>
      <nav aria-label={t('agentChat.history.projects')} className="min-h-0 flex-1 space-y-0.5 overflow-y-auto p-1.5">
        {projects.map((project) => {
          const selected = project.path === selectedProjectPath
          const current = project.path === currentProjectPath
          const name = project.name || project.path
          return (
            <button
              key={project.path}
              type="button"
              onClick={() => onSelect(project.path)}
              aria-current={selected ? 'true' : undefined}
              aria-label={t('agentChat.history.projectSessionCount', { name, count: project.total })}
              title={project.path}
              className={`relative flex w-full min-w-0 items-center gap-1.5 rounded-[var(--nova-radius)] px-2 py-1.5 text-left outline-none transition-colors focus-visible:ring-1 focus-visible:ring-[var(--nova-accent)] ${
                selected
                  ? 'bg-[var(--nova-active)] text-[var(--nova-text)]'
                  : 'text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]'
              }`}
            >
              <span
                aria-hidden="true"
                className={`absolute bottom-1.5 left-0 top-1.5 w-0.5 rounded-full ${selected ? 'bg-[var(--nova-text)]' : 'bg-transparent'}`}
              />
              <Folder aria-hidden="true" className="size-3.5 shrink-0 text-[var(--nova-text-faint)]" />
              <span className="min-w-0 flex-1 truncate text-[11px]">{name}</span>
              {current ? (
                <span
                  aria-label={t('agentChat.history.currentProject')}
                  title={t('agentChat.history.currentProject')}
                  className="size-1.5 shrink-0 rounded-full bg-[var(--nova-text-muted)]"
                />
              ) : null}
              <span className="shrink-0 text-[9px] tabular-nums text-[var(--nova-text-faint)]">{project.total}</span>
            </button>
          )
        })}
      </nav>
    </aside>
  )
}

/** Compact project switcher used when the master pane cannot fit beside the session list. */
export function AgentChatHistoryProjectSelect({
  projects,
  selectedProjectPath,
  onSelect,
}: HistoryProjectNavProps) {
  const { t } = useTranslation()
  return (
    <Select value={selectedProjectPath} onValueChange={onSelect} disabled={projects.length === 0}>
      <SelectTrigger size="sm" className="h-8 w-full bg-[var(--nova-surface-2)] text-xs" aria-label={t('agentChat.history.selectProject')}>
        <SelectValue placeholder={t('agentChat.history.selectProject')} />
      </SelectTrigger>
      <SelectContent>
        {projects.map((project) => (
          <SelectItem key={project.path} value={project.path}>
            {project.name || project.path} · {project.total}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}

export function currentProjectFirst(
  projects: readonly AgentChatProject[],
  currentProjectPath: string,
): AgentChatProject[] {
  const ordered = [...projects]
  const currentIndex = ordered.findIndex((project) => project.path === currentProjectPath)
  if (currentIndex <= 0) return ordered
  const [current] = ordered.splice(currentIndex, 1)
  return current ? [current, ...ordered] : ordered
}
