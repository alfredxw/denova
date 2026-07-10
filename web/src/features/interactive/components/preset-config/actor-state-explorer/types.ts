import type { ActorStateField, ActorStateInitialActor, ActorStateTemplate, ActorTraitDefinition, ActorTraitPool, StoryDirectorActorStateSystem } from '../../../types'

export type TreeNodeKind =
  | 'group'
  | 'template'
  | 'field'
  | 'actors-group'
  | 'actor'
  | 'trait-library'
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
  | { kind: 'pool'; pool: ActorTraitPool; poolIndex: number }
  | { kind: 'trait'; trait: ActorTraitDefinition; traitIndex: number; pool: ActorTraitPool; poolIndex: number }

export interface ExplorerProps {
  value: StoryDirectorActorStateSystem
  onChange: (value: ExplorerProps['value']) => void
  onValidityChange?: (valid: boolean) => void
}

export interface SelectionState {
  selectedId: string
  expandedIds: Set<string>
}
