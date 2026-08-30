import type { BranchPlan, Snapshot } from '../../types'
import { StateDisplayPreferenceMenu } from '../story-state/StateDisplayPreferenceMenu'
import type { StoryStateDisplayPreference } from '../story-state/display-preference'
import { BranchPlanSummary } from './BranchPlanView'
import { StateView } from './StateView'

interface OverviewViewProps {
  snapshot: Snapshot | null
  stateFacts: Array<[string, unknown]>
  stateError?: string
  plan?: BranchPlan
  planningEnabled: boolean
  stateDisplayPreference: StoryStateDisplayPreference
  onStateDisplayPreferenceChange: (value: StoryStateDisplayPreference) => void
}

export function OverviewView({ snapshot, stateFacts, stateError, plan, planningEnabled, stateDisplayPreference, onStateDisplayPreferenceChange }: OverviewViewProps) {
  return (
    <div className="director-console__scroll h-full min-h-0 overflow-y-auto px-3 py-3">
      <div className="mb-3 flex justify-end">
        <StateDisplayPreferenceMenu value={stateDisplayPreference} onChange={onStateDisplayPreferenceChange} compact />
      </div>
      <div className="flex flex-col gap-5">
        <BranchPlanSummary plan={plan} planningEnabled={planningEnabled} />
        <StateView snapshot={snapshot} stateFacts={stateFacts} syncError={stateError} section="overview" />
      </div>
    </div>
  )
}
