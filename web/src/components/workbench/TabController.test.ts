import { beforeEach, describe, expect, it } from 'vitest'
import {
  enforceTabLimit,
  persistTabsFor,
  readTabsFor,
  WRITING_SUBAGENT_TAB_KEY,
  type Tab,
} from './TabController'

const subAgentTab: Tab = {
  kind: 'subagent',
  parentSessionId: 'parent-session',
  sessionKey: 'child-session',
  title: 'Researcher',
  returnTabKey: 'file:chapter.md',
}

describe('writing workbench temporary SubAgent tab', () => {
  beforeEach(() => window.localStorage.clear())

  it('does not persist the temporary tab or replace the durable active-tab preference', () => {
    persistTabsFor('/book', [{ kind: 'file', path: 'chapter.md' }, subAgentTab])

    expect(readTabsFor('/book')).toEqual([{ kind: 'file', path: 'chapter.md' }])
    expect(window.localStorage.getItem('nova.layout.tabs:/book')).not.toContain(WRITING_SUBAGENT_TAB_KEY)
  })

  it('does not count the temporary tab against the durable tab limit', () => {
    const tabs = enforceTabLimit(
      [{ kind: 'file', path: 'one.md' }, { kind: 'file', path: 'two.md' }, subAgentTab],
      WRITING_SUBAGENT_TAB_KEY,
      2,
      new Map(),
    )

    expect(tabs).toHaveLength(3)
    expect(tabs).toContainEqual(subAgentTab)
  })
})
