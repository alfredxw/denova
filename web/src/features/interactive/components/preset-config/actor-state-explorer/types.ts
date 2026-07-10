import type { ActorStateField, ActorStateInitialActor, ActorStateTemplate, OpeningTrait, OpeningTraitPool, StateOp } from '../../../types'

export type TreeNodeKind =
  | 'group'
  | 'template'
  | 'field'
  | 'actor-group'
  | 'actor'
  | 'opening-group'
  | 'opening-ops'
  | 'pool'
  | 'trait'

export interface TreeNode {
  id: string
  kind: TreeNodeKind
  label: string
  subtitle?: string
  badge?: string
  selectable: boolean
  children: TreeNode[]
  /** Data payload for selectable nodes */
  data?: TreeNodeData
}

export type TreeNodeData =
  | { kind: 'template'; template: ActorStateTemplate; index: number }
  | { kind: 'field'; field: ActorStateField; fieldIndex: number; template: ActorStateTemplate; templateIndex: number }
  | { kind: 'actor'; actor: ActorStateInitialActor; actorIndex: number; template?: ActorStateTemplate }
  | { kind: 'opening-ops'; ops: StateOp[] }
  | { kind: 'pool'; pool: OpeningTraitPool; poolIndex: number }
  | { kind: 'trait'; trait: OpeningTrait; traitIndex: number; pool: OpeningTraitPool; poolIndex: number }

export interface ExplorerProps {
  value: {
    templates?: ActorStateTemplate[]
    initial_actors?: ActorStateInitialActor[]
    trait_pools?: OpeningTraitPool[]
    initial_state_ops?: StateOp[]
    opening_enabled?: boolean
  }
  onChange: (value: ExplorerProps['value']) => void
  onValidityChange?: (valid: boolean) => void
}

export interface SelectionState {
  selectedId: string
  expandedIds: Set<string>
}
