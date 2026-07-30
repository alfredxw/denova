import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { AgentApprovalMode, LayeredSettings } from '@/features/settings/types'
import { AgentApprovalProvider, useAgentApprovalMode } from './AgentApprovalProvider'

const fetchSettings = vi.fn()
const updateAgentApprovalMode = vi.fn()

vi.mock('@/features/settings/api', () => ({
  fetchSettings: (...args: unknown[]) => fetchSettings(...args),
  updateAgentApprovalMode: (...args: unknown[]) => updateAgentApprovalMode(...args),
}))

describe('AgentApprovalProvider', () => {
  beforeEach(() => {
    fetchSettings.mockReset()
    updateAgentApprovalMode.mockReset()
  })

  it('requires a user-level first choice with Write preselected', async () => {
    const user = userEvent.setup()
    fetchSettings.mockResolvedValue(settingsSnapshot(undefined))
    updateAgentApprovalMode.mockResolvedValue(settingsSnapshot('write'))

    render(
      <AgentApprovalProvider>
        <ModeProbe />
      </AgentApprovalProvider>,
    )

    const dialog = await screen.findByRole('dialog')
    expect(screen.getByRole('radio', { name: /Write/ })).toHaveAttribute('aria-checked', 'true')
    expect(dialog).toHaveTextContent('这是执行前安全护栏')
    await user.click(screen.getByRole('button', { name: '保存并继续' }))

    await waitFor(() => expect(updateAgentApprovalMode).toHaveBeenCalledWith('write'))
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
    expect(screen.getByTestId('mode')).toHaveTextContent('write')
  })

  it('does not reopen onboarding when the user already chose a mode', async () => {
    fetchSettings.mockResolvedValue(settingsSnapshot('ask'))
    render(
      <AgentApprovalProvider>
        <ModeProbe />
      </AgentApprovalProvider>,
    )
    await waitFor(() => expect(screen.getByTestId('mode')).toHaveTextContent('ask:true'))
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })
})

function ModeProbe() {
  const approval = useAgentApprovalMode()
  return <span data-testid="mode">{approval.mode}:{String(approval.initialized)}</span>
}

function settingsSnapshot(userMode: AgentApprovalMode | undefined): LayeredSettings {
  return {
    default: { agent_approval_mode: 'ask' },
    global: {},
    user: userMode ? { agent_approval_mode: userMode } : {},
    workspace: {},
    effective: { agent_approval_mode: userMode || 'ask' },
    paths: { denova_dir: '/tmp', nova_dir: '/tmp', user_config: '/tmp/config.toml', workspace_config: '' },
    resolved_agent_tool_manifests: {},
    resolved_agent_contexts: {},
  }
}
