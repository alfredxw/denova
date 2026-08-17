import { useId, useState } from 'react'
import { Bot, Check, CircleAlert, FolderOpen } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {
  WorkbenchContextSwitcherTrigger,
  WORKBENCH_CONTEXT_MENU_CLASS,
  WORKBENCH_CONTEXT_MENU_GROUP_CLASS,
  WORKBENCH_CONTEXT_MENU_ITEM_CLASS,
} from '@/components/workbench/WorkbenchContextSwitcher'
import { cn } from '@/lib/utils'
import type { AgentChatProject } from './api'

export interface AgentChatProjectNavigationState {
  projects: AgentChatProject[]
  activeProjectId: string
  loading: boolean
  selectProject: (projectId: string) => void
}

interface AgentChatProjectSwitcherProps {
  navigation: AgentChatProjectNavigationState | null
  compact?: boolean
}

/** AgentChat-only project context switcher; changing it never changes the foreground Book. */
export function AgentChatProjectSwitcher({ navigation, compact = false }: AgentChatProjectSwitcherProps) {
  const { t } = useTranslation()
  const menuLabelID = useId()
  const [open, setOpen] = useState(false)
  const projects = navigation?.projects ?? []
  const activeProject = projects.find((project) => project.id === navigation?.activeProjectId) ?? null
  const loading = navigation === null || navigation.loading
  const label = activeProject?.name || activeProject?.path || t(
    loading ? 'agentChat.projectSwitcher.loading' : 'agentChat.projectSwitcher.empty',
  )
  const TriggerIcon = activeProject?.type === 'general' ? Bot : FolderOpen

  const selectProject = (project: AgentChatProject) => {
    if (!navigation || project.id === navigation.activeProjectId) {
      setOpen(false)
      return
    }
    navigation.selectProject(project.id)
    setOpen(false)
  }

  return (
    <DropdownMenu open={open} onOpenChange={setOpen}>
      <DropdownMenuTrigger asChild>
        <WorkbenchContextSwitcherTrigger
          aria-label={t('agentChat.projectSwitcher.trigger', { name: label })}
          icon={TriggerIcon}
          label={label}
          compact={compact}
          disabled={projects.length === 0}
          aria-busy={loading}
        />
      </DropdownMenuTrigger>
      <DropdownMenuContent
        align="start"
        sideOffset={6}
        collisionPadding={8}
        aria-labelledby={menuLabelID}
        className={WORKBENCH_CONTEXT_MENU_CLASS}
      >
        <div id={menuLabelID} className="shrink-0 px-3 pb-2 pt-3 text-[11px] font-medium text-[var(--nova-text-faint)]">
          {t('agentChat.projectSwitcher.title')}
        </div>
        <DropdownMenuGroup className={WORKBENCH_CONTEXT_MENU_GROUP_CLASS}>
          {projects.map((project) => {
            const current = project.id === navigation?.activeProjectId
            const ProjectIcon = project.type === 'general' ? Bot : FolderOpen
            return (
              <DropdownMenuItem
                key={project.id}
                aria-current={current ? 'page' : undefined}
                className={cn(WORKBENCH_CONTEXT_MENU_ITEM_CLASS, current && 'bg-[var(--nova-active)]')}
                onSelect={() => selectProject(project)}
              >
                <span className="flex h-9 w-8 shrink-0 items-center justify-center rounded-[4px] border border-[var(--nova-border)] bg-[var(--nova-surface)]">
                  <ProjectIcon aria-hidden="true" className="h-4 w-4 text-[var(--nova-text-muted)]" />
                </span>
                <span className="min-w-0 flex-1">
                  <span className="block truncate text-xs font-medium text-[var(--nova-text)]">{project.name || project.path}</span>
                  <span className="mt-0.5 block truncate text-[10px] text-[var(--nova-text-faint)]">
                    {t('agentChat.projectSwitcher.summary', { count: project.total, path: project.path })}
                  </span>
                </span>
                {project.status === 'missing' ? (
                  <CircleAlert aria-label={t('agentChat.project.missing')} className="h-3.5 w-3.5 text-[var(--nova-warning)]" />
                ) : current ? (
                  <Check aria-hidden="true" className="h-3.5 w-3.5 text-[var(--nova-text-muted)]" />
                ) : null}
              </DropdownMenuItem>
            )
          })}
        </DropdownMenuGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
