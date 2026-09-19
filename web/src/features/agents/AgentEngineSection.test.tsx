import { useState } from 'react'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import type { AgentEngineID, RuntimePreferences } from '@/features/agent-runtime/types'
import { AgentEngineSection } from './AgentEngineSection'

const mocks = vi.hoisted(() => ({
  fetchAgentEngines: vi.fn(), fetchEngineModels: vi.fn(),
  profiles: [] as { id: string; label: string; modelLabel: string }[],
}))
vi.mock('@/features/agent-runtime/api', () => ({ ...mocks, checkAgentEngine: vi.fn() }))
vi.mock('@/features/agent-runtime/api-profiles', () => ({ useRuntimeProfiles: () => ({ profiles: mocks.profiles, failed: false }) }))

function Harness({ engine, onChange }: { engine: AgentEngineID; onChange: (value: RuntimePreferences) => void }) {
  const [value, setValue] = useState<RuntimePreferences>({ selected: engine })
  return <AgentEngineSection value={value} inherited={{ [engine]: { model: 'cli-model', effort: 'high', ...(engine === 'codex' ? { sandbox: 'read-only' } : {}) } }}
    onChange={next => { onChange(next); setValue(next) }} beforeSwitch={async () => {}} onCatalog={() => {}} />
}

describe('AgentEngineSection model source', () => {
  for (const engine of ['codex', 'claude'] as const) {
    it(`switches ${engine} inherited CLI settings to a Denova profile and back atomically`, async () => {
      mocks.fetchAgentEngines.mockResolvedValue({ items: [{ id: engine, name_key: `agentRuntime.${engine}`, status: 'ready' }] })
      mocks.fetchEngineModels.mockResolvedValue({ items: [{ id: 'cli-model', display_name: 'CLI model', efforts: ['high'] }], default_id: 'cli-model' })
      mocks.profiles = [{ id: 'profile:gateway', label: 'Gateway', modelLabel: 'Gateway' }]
      const onChange = vi.fn()
      render(<Harness engine={engine} onChange={onChange} />)
      await userEvent.click(screen.getByRole('radio', { name: 'Denova 模型' }))
      expect(onChange).toHaveBeenLastCalledWith({ selected: engine, [engine]: { profile_id: 'gateway', ...(engine === 'codex' ? { sandbox: 'read-only' } : {}) } })
      expect(screen.queryByRole('combobox', { name: '推理强度' })).not.toBeInTheDocument()
      await waitFor(() => expect(screen.getByRole('radio', { name: 'CLI 模型' })).toBeEnabled())
      await userEvent.click(screen.getByRole('radio', { name: 'CLI 模型' }))
      expect(onChange).toHaveBeenLastCalledWith({ selected: engine, [engine]: { model: 'cli-model', ...(engine === 'codex' ? { sandbox: 'read-only' } : {}) } })
    })
  }

  it('explains the required protocol when no compatible Denova model exists', async () => {
    mocks.fetchAgentEngines.mockResolvedValue({ items: [{ id: 'claude', name_key: 'agentRuntime.claude', status: 'auth_required' }] })
    mocks.profiles = []
    const onChange = vi.fn()
    render(<Harness engine="claude" onChange={onChange} />)
    expect(screen.getByRole('radio', { name: 'Denova 模型' })).toBeDisabled()
    expect(screen.getByText(/请在设置 → 模型中添加使用 Anthropic Messages/)).toBeVisible()
    expect(onChange).not.toHaveBeenCalled()
    await screen.findByText('需要登录')
  })
})
