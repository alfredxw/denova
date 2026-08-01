import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { AgentApprovalMode, LayeredSettings } from '@/features/settings/types'
import { AgentApprovalProvider, useAgentApprovalMode } from './AgentApprovalProvider'

const fetchSettings = vi.fn()
const updateAgentApprovalMode = vi.fn()

vi.mock('@/features/settings/api', () => ({
  fetchSettings: (...args: unknown[]) => fetchSettings(...args),
  patchSettings: (_layer: string, changes: { agent_approval_mode?: AgentApprovalMode }) => updateAgentApprovalMode(changes.agent_approval_mode),
}))

describe('AgentApprovalProvider', () => {
  beforeEach(() => {
    fetchSettings.mockReset()
    updateAgentApprovalMode.mockReset()
  })

  it('uses the effective Write default without prompting or persisting it', async () => {
    fetchSettings.mockResolvedValue(settingsSnapshot(undefined))

    render(
      <AgentApprovalProvider>
        <ModeProbe />
      </AgentApprovalProvider>,
    )

    await waitFor(() => expect(screen.getByTestId('mode')).toHaveTextContent('write:true'))
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(updateAgentApprovalMode).not.toHaveBeenCalled()
  })

  it('keeps an existing explicit user mode without prompting', async () => {
    fetchSettings.mockResolvedValue(settingsSnapshot('ask'))
    render(
      <AgentApprovalProvider>
        <ModeProbe />
      </AgentApprovalProvider>,
    )
    await waitFor(() => expect(screen.getByTestId('mode')).toHaveTextContent('ask:true'))
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })

  it('blocks initialization on load failure and recovers through retry', async () => {
    const user = userEvent.setup()
    fetchSettings
      .mockRejectedValueOnce(new Error('offline'))
      .mockResolvedValueOnce(settingsSnapshot(undefined))

    render(
      <AgentApprovalProvider>
        <ModeProbe />
      </AgentApprovalProvider>,
    )

    expect(await screen.findByRole('dialog')).toHaveTextContent('无法加载安全模式')
    expect(screen.getByTestId('mode')).toHaveTextContent('write:false')
    await user.click(screen.getByRole('button', { name: '重试' }))

    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
    expect(screen.getByTestId('mode')).toHaveTextContent('write:true')
  })
})

function ModeProbe() {
  const approval = useAgentApprovalMode()
  return <span data-testid="mode">{approval.mode}:{String(approval.initialized)}</span>
}

function settingsSnapshot(userMode: AgentApprovalMode | undefined): LayeredSettings {
  const effectiveMode = userMode || 'write'
  return {
    default: { agent_approval_mode: 'write' },
    global: {},
    user: userMode ? { agent_approval_mode: userMode as AgentApprovalMode } : {},
    workspace: {},
    effective: { agent_approval_mode: effectiveMode as AgentApprovalMode },
    paths: { denova_dir: '/tmp', nova_dir: '/tmp', user_config: '/tmp/config.toml', workspace_config: '' },
    resolved_agent_tool_manifests: {},
    resolved_agent_contexts: {},
  }
}
