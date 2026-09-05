import { beforeEach, describe, expect, it } from 'vitest'
import { appendTab, persistWorkbenchState, readStoredWorkbenchState } from './tab-state'
import type { AgentChatSubAgentTab, AgentChatTab } from './types'

const parent: AgentChatTab = {
  kind: 'agent',
  id: 'parent-tab',
  projectId: 'project-a',
  workspace: '/books/a',
  group: 'primary',
  sessionId: 'parent-session',
}

const child: AgentChatSubAgentTab = {
  kind: 'subagent',
  id: 'child-tab',
  projectId: 'project-a',
  workspace: '/books/a',
  group: 'secondary',
  parentTabId: parent.id,
  parentSessionId: 'parent-session',
  sessionKey: 'child-one',
  title: 'Researcher',
}

describe('Agent Chat temporary SubAgent tabs', () => {
  beforeEach(() => window.localStorage.clear())

  it('reuses one child tab per parent and updates its selected session', () => {
    const appended = appendTab([parent, child], {
      ...child,
      id: 'discarded-new-id',
      sessionKey: 'child-two',
      title: 'Reviewer',
    })

    expect(appended.activeId).toBe(child.id)
    expect(appended.tabs.filter((tab) => tab.kind === 'subagent')).toEqual([
      expect.objectContaining({ id: child.id, sessionKey: 'child-two', title: 'Reviewer' }),
    ])
  })

  it('omits child tabs from restored workbench state', () => {
    persistWorkbenchState({
      activeProjectId: 'project-a',
      projects: {
        'project-a': {
          tabs: [parent, child],
          activeTabIds: { primary: parent.id, secondary: child.id },
          focusedGroup: 'secondary',
          secondaryVisible: true,
        },
      },
    })

    expect(readStoredWorkbenchState().projects['project-a']).toEqual(expect.objectContaining({
      tabs: [parent],
      activeTabIds: { primary: parent.id, secondary: null },
      secondaryVisible: false,
    }))
  })
})
