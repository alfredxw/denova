import { Hash, Route } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { TooltipIconButton } from '@/components/common/tooltip-icon-button'
import { useTrajectoryNavigation } from '@/features/trajectory/trajectory-navigation'
import { ContextCopyButton } from './ContextCopyButton'

const actionClassName = 'size-5 border border-transparent bg-transparent text-[var(--nova-text-faint)] shadow-none hover:border-[var(--nova-border)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text-muted)] [&_svg]:size-3'

/** Actions target this reply's Run, including unfinished runs without terminal prose. */
export function AgentRunActions({ projectId, runID }: { projectId?: string; runID: string }) {
  const { t } = useTranslation()
  const trajectory = useTrajectoryNavigation()
  if (!runID) return null

  return (
    <>
      <ContextCopyButton
        key={runID}
        content={runID}
        icon={Hash}
        label={t('chat.action.copyRunId')}
        copiedLabel={t('chat.action.runIdCopied')}
        failedLabel={t('chat.action.copyRunIdFailed')}
        className={actionClassName}
      />
      {projectId && trajectory.enabled ? (
        <TooltipIconButton
          label={t('trajectory.openRun')}
          tooltipSide="top"
          tooltipSideOffset={3}
          className={actionClassName}
          onClick={() => trajectory.open({ projectId, runId: runID })}
        >
          <Route />
        </TooltipIconButton>
      ) : null}
    </>
  )
}
