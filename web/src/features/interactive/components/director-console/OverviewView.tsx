import { useMemo } from 'react'
import { Activity } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { InlineErrorNotice } from '@/components/common/inline-error-notice'
import type { BranchPlan, Snapshot } from '../../types'
import { ChangesSummary } from '../story-state/ChangesSummary'
import { buildStoryStateModel, type ActorStateEntry } from '../story-state/model'
import { BranchPlanSummary } from './BranchPlanView'
import { StateDetailsDialog } from './StateDetailsDialog'

interface OverviewViewProps {
  snapshot: Snapshot | null
  stateError?: string
  plan?: BranchPlan
  planningEnabled: boolean
  branchPlanEditingDisabled?: boolean
  onBranchPlanUpdate?: (markdown: string, baseRevision: string) => void | Promise<void>
}

export function OverviewView({ snapshot, stateError, plan, planningEnabled, branchPlanEditingDisabled = false, onBranchPlanUpdate }: OverviewViewProps) {
  const { t } = useTranslation()
  const model = useMemo(() => buildStoryStateModel(snapshot), [snapshot])
  const actors = useMemo<ActorStateEntry[]>(() => [
    ...model.actors,
    ...model.archivedActors.map((entry): ActorStateEntry => [entry.actorId, { name: entry.name, template_id: entry.templateId }]),
  ], [model.actors, model.archivedActors])
  const error = snapshot?.current_turn?.state_error || stateError

  return (
    <div className="director-console__scroll h-full min-h-0 overflow-y-auto px-3 py-3">
      <div className="flex flex-col gap-3">
        <BranchPlanSummary
          plan={plan}
          planningEnabled={planningEnabled}
          editingDisabled={branchPlanEditingDisabled}
          onUpdate={onBranchPlanUpdate}
        />
        {error ? <InlineErrorNotice message={error} /> : null}
        <section className="story-state-ledger overflow-hidden rounded-xl border border-[var(--nova-border)] bg-[var(--director-panel)]">
          {model.changes.length > 0 ? (
            <ChangesSummary changes={model.changes} actors={actors} schema={snapshot?.actor_state_schema} standalone />
          ) : (
            <div className="flex items-center gap-2 px-3 py-2.5 text-[10px] text-[var(--nova-text-faint)]">
              <Activity className="size-3.5" />
              <span>{t('directorPanel.stateDeltaEmpty')}</span>
            </div>
          )}
        </section>
        <StateDetailsDialog snapshot={snapshot} stateError={stateError} />
      </div>
    </div>
  )
}
