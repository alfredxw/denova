import { create } from 'zustand'
import { createAgentChatSession, getAgentChatProjects } from '@/features/agent-chat/api'
import { readAgentChatActiveSession } from '@/features/agent-chat/session-preferences'
import { requestAgentChatSessionNavigation } from '@/features/agent-chat/session-navigation'
import { useWorkspaceStore } from '@/stores/workspace-store'
import { type DevelopmentSource, type PackageKind } from './api'
import { sourceManifestPath } from './extension-directory'
import { useDevelopmentContext } from './development-context'

/** Cross-page selection is transient; installed records and Projects own their identities. */
export const useExtensionNavigation = create<{ selected: string }>(() => ({ selected: '' }))

/** One-shot navigation intent, retained until the game destination has loaded its catalog. */
export const useGameCreationRequest = create<{ gameId: string | null }>(() => ({ gameId: null }))

export function startExtensionGame(gameId: string) {
  useGameCreationRequest.setState({ gameId })
  useWorkspaceStore.getState().setMode('interactive')
  console.info('[extensions] Opening new story setup', { gameId })
}

export function openInstalledExtension(kind: PackageKind, id: string) {
  useExtensionNavigation.setState({ selected: `installed:${kind}:${id}` })
  useWorkspaceStore.getState().setMode('extensions')
}

export async function openExtensionSource(source: DevelopmentSource) {
  const project = (await getAgentChatProjects()).find(project => project.id === source.projectId)
  if (!project) throw new Error('Extension source Project is unavailable')
  const preferred = readAgentChatActiveSession(project.id)
  const sessionId = project.sessions.find(session => session.id === preferred)?.id
    ?? project.sessions[0]?.id ?? (await createAgentChatSession(project.id, '')).id
  useDevelopmentContext.getState().select(project.id, source.developmentId)
  requestAgentChatSessionNavigation({ projectId: project.id, sessionId, sourcePath: sourceManifestPath(source) })
  useWorkspaceStore.getState().setMode('agentchat')
}
