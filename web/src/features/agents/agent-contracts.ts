import type { AgentContractID, AgentRuntimeKind } from '@/features/settings/types'

export const AGENT_CONTRACT_BY_KIND: Record<AgentRuntimeKind, AgentContractID> = {
  general: 'project.general.v1',
  ide: 'writing.primary.v1',
  interactive_story: 'game.narrator.v1',
  image: 'image.creator.v1',
}

export function contractForRuntimeKind(kind: AgentRuntimeKind): AgentContractID {
  return AGENT_CONTRACT_BY_KIND[kind]
}

export function runtimeKindForContract(contract?: string): AgentRuntimeKind | undefined {
  return (Object.entries(AGENT_CONTRACT_BY_KIND) as Array<[AgentRuntimeKind, AgentContractID]>)
    .find(([, id]) => id === contract)?.[0]
}
