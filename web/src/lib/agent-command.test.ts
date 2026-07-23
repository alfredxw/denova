import { describe, expect, it, vi } from 'vitest'
import type { TFunction } from 'i18next'
import { agentCommandErrorMessage, rememberAgentCommandID } from './agent-command'

describe('rememberAgentCommandID', () => {
  it('reuses an uncertain identity and bounds retained retry state', () => {
    const commands = new Map<string, string>()
    const create = vi.fn()
      .mockReturnValueOnce('command-1')
      .mockReturnValueOnce('command-2')
      .mockReturnValueOnce('command-3')

    expect(rememberAgentCommandID(commands, 'request-1', create, 2)).toBe('command-1')
    expect(rememberAgentCommandID(commands, 'request-1', create, 2)).toBe('command-1')
    expect(rememberAgentCommandID(commands, 'request-2', create, 2)).toBe('command-2')
    expect(rememberAgentCommandID(commands, 'request-3', create, 2)).toBe('command-3')

    expect(create).toHaveBeenCalledTimes(3)
    expect([...commands.entries()]).toEqual([
      ['request-2', 'command-2'],
      ['request-3', 'command-3'],
    ])
  })
})

describe('agentCommandErrorMessage', () => {
  it('maps an idempotency conflict through the localized invalid-command copy', () => {
    const t = vi.fn((key: string) => key) as unknown as TFunction

    expect(agentCommandErrorMessage({ code: 'agent_runtime.command_conflict' }, t)).toBe('chat.runtime.invalidCommand')
  })
})
