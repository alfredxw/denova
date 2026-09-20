import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, it, vi } from 'vitest'
import { ApplyAgentEngineSection } from './ApplyAgentEngineSection'

const patch = vi.hoisted(() => vi.fn().mockResolvedValue(true))
vi.mock('@/features/conversation-config/use-conversation-config', () => ({ useConversationConfig: () => ({
  snapshot: { revision: 1, agent_kind: 'ide', runtime: { kind: 'claude', claude: { model: 'sonnet' } } },
  patch, loading: false, saving: false, error: null,
}) }))

it('applies a model source within the same runtime through the model patch', async () => {
  const save = vi.fn().mockResolvedValue(undefined)
  render(<ApplyAgentEngineSection binding={{ mode: 'writing', session_id: 'session' }} agentKind="ide"
    runtime={{ selected: 'claude', claude: { profile_id: 'gateway' } }} saveDefaults={save} />)
  await userEvent.click(screen.getByRole('button', { name: '应用到此会话' }))
  expect(save).toHaveBeenCalled()
  expect(patch).toHaveBeenLastCalledWith({ claude: { profile_id: 'gateway' } })
  expect(await screen.findByRole('status')).toHaveTextContent('已应用。')
})

it('keeps the runtime switch contract when changing engines', async () => {
  render(<ApplyAgentEngineSection binding={{ mode: 'writing', session_id: 'session' }} agentKind="ide"
    runtime={{ selected: 'codex', codex: { profile_id: 'gateway', sandbox: 'read-only' } }} saveDefaults={async () => {}} />)
  await userEvent.click(screen.getByRole('button', { name: '应用到此会话' }))
  expect(patch).toHaveBeenLastCalledWith({ runtime: { kind: 'codex', codex: { profile_id: 'gateway', sandbox: 'read-only' } } })
})
