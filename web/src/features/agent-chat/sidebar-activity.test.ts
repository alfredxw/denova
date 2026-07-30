import { describe, expect, it } from 'vitest'
import type { AgentChatProject } from './api'
import { projectSidebarActivities, summarizeSidebarActivities } from './sidebar-activity'
import type { AgentChatProjectTabState } from './tab-state'

const project: AgentChatProject = {
  id: 'project-alpha',
  type: 'book',
  status: 'available',
  path: '/books/alpha',
  name: 'Alpha',
  current: true,
  total: 3,
  sessions: [
    {
      id: 'open',
      title: 'Open chat',
      created_at: '',
      updated_at: '',
      message_count: 2,
      running: true,
      active: false,
    },
    {
      id: 'detached',
      title: 'Detached task',
      created_at: '',
      updated_at: '',
      message_count: 3,
      running: true,
      active: false,
    },
    {
      id: 'history',
      title: 'History only',
      created_at: '',
      updated_at: '',
      message_count: 4,
      running: false,
      active: false,
    },
  ],
}

const state: AgentChatProjectTabState = {
  tabs: [
    {
      kind: 'agent',
      id: 'agent-tab',
      projectId: project.id,
      workspace: project.path,
      sessionId: 'open',
      group: 'primary',
    },
    {
      kind: 'page',
      id: 'reader-tab',
      projectId: project.id,
      workspace: project.path,
      pageId: 'reader',
      group: 'primary',
    },
    {
      kind: 'terminal',
      id: 'terminal-tab',
      projectId: project.id,
      workspace: project.path,
      profileId: 'shell',
      title: '',
      group: 'secondary',
    },
    {
      kind: 'review',
      id: 'review-tab',
      projectId: project.id,
      workspace: project.path,
      threadID: 'review',
      group: 'secondary',
    },
  ],
  activeTabIds: { primary: 'agent-tab', secondary: 'terminal-tab' },
  focusedGroup: 'secondary',
}

describe('AgentChat sidebar activity projection', () => {
  it('filters tab order to conversations and terminals, then appends detached running work', () => {
    const activities = projectSidebarActivities({
      project,
      state,
      activeProjectId: project.id,
      runningSessionIds: new Set(['open', 'detached']),
      terminalStatuses: new Map([['terminal-tab', 'ready']]),
      tabTitle: (tab) => tab.id,
    })

    expect(activities.map((activity) => activity.id)).toEqual(['agent:open', 'terminal:terminal-tab', 'agent:detached'])
    expect(activities[0]).toMatchObject({
      status: 'running',
      paneVisible: true,
      focused: false,
    })
    expect(activities[1]).toMatchObject({
      status: 'ready',
      paneVisible: true,
      focused: true,
    })
    expect(activities[2]).toMatchObject({
      status: 'running',
      paneVisible: false,
    })
    expect(activities[2]).not.toHaveProperty('tabId')
  })

  it('does not mark a retained pane selection as visible while another project is foreground', () => {
    const [activity] = projectSidebarActivities({
      project,
      state,
      activeProjectId: 'project-other',
      runningSessionIds: new Set(),
      terminalStatuses: new Map(),
      tabTitle: () => 'Chat',
    })
    expect(activity).toMatchObject({ paneVisible: false, focused: false })
  })

  it('summarizes running and error states without classifying a normal exit as failure', () => {
    const activities = projectSidebarActivities({
      project,
      state,
      activeProjectId: project.id,
      runningSessionIds: new Set(['open', 'detached']),
      terminalStatuses: new Map([['terminal-tab', 'error']]),
      tabTitle: (tab) => tab.id,
    })
    expect(summarizeSidebarActivities(activities)).toEqual({
      total: 3,
      running: 2,
      attention: 1,
    })
  })
})
