import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fetchSettings } from '@/features/settings/api'
import type { LayeredSettings } from '@/features/settings/types'
import type { ConversationConfigChanges, ConversationConfigSnapshot } from '@/features/conversation-config/types'
import { ModelProfileSwitcher } from './ModelProfileSwitcher'

vi.mock('@/features/settings/api', () => ({
  fetchSettings: vi.fn(),
}))

let latestSettings: LayeredSettings
const patchConversationConfig = vi.fn<(changes: ConversationConfigChanges) => void>()

describe('ModelProfileSwitcher quick control', () => {
  beforeEach(() => {
    latestSettings = settingsSnapshot({
      user: { agent_models: { ide: { profile_id: 'fast', thinking_level: 'medium' } } },
      effective: {
        model_profiles: [
          { id: 'default', name: 'GPT 4.1', model: 'gpt-4.1' },
          { id: 'fast', name: 'Turbo', model: 'gpt-4.1-mini' },
        ],
        agent_models: { ide: { profile_id: 'fast', thinking_level: 'medium' } },
      },
    })
    vi.mocked(fetchSettings).mockReset()
    vi.mocked(fetchSettings).mockImplementation(async () => latestSettings)
    patchConversationConfig.mockReset()
  })

  it('uses a borderless text-and-chevron trigger with the current thinking level', async () => {
    const { container } = render(<SwitcherHarness />)

    const trigger = await screen.findByRole('button', { name: '切换模型，当前：Turbo 中' })
    expect(trigger).toHaveAttribute('data-current-model', 'Turbo')
    expect(trigger).toHaveAttribute('data-current-thinking-level', 'medium')
    expect(trigger).toHaveClass('border-0', 'bg-transparent')
    expect(container.querySelector('.lucide-cpu')).not.toBeInTheDocument()
    expect(container.querySelectorAll('svg')).toHaveLength(1)
    expect(container.querySelector('.lucide-chevron-down')).toBeInTheDocument()
  })

  it('switches the model from its popup list', async () => {
    const user = userEvent.setup()
    render(<SwitcherHarness />)

    const trigger = await screen.findByRole('button', { name: '切换模型，当前：Turbo 中' })
    expect(trigger).toHaveAttribute('data-current-model', 'Turbo')

    await user.click(trigger)
    expect(screen.getByText('模型')).toBeInTheDocument()
    expect(screen.getByText('思考强度')).toBeInTheDocument()
    expect(screen.getByRole('group', { name: '思考强度' })).toHaveClass('grid-cols-4')
    await user.click(screen.getByRole('menuitem', { name: '默认：GPT 4.1' }))

    await waitFor(() => expect(patchConversationConfig).toHaveBeenCalledWith({ profile_id: 'default' }))
    expect(await screen.findByRole('button', { name: '切换模型，当前：GPT 4.1 中' })).toBeInTheDocument()
  })

  it('persists an explicit per-conversation thinking level', async () => {
    const user = userEvent.setup()
    render(<SwitcherHarness />)

    await user.click(await screen.findByRole('button', { name: '切换模型，当前：Turbo 中' }))
    await user.click(screen.getByRole('button', { name: '最大' }))

    await waitFor(() => expect(patchConversationConfig).toHaveBeenLastCalledWith({ thinking_level: 'max' }))
    const maxTrigger = await screen.findByRole('button', { name: '切换模型，当前：Turbo 最大' })
    expect(maxTrigger).toHaveAttribute('data-current-thinking-level', 'max')
  })

  it('keeps user-scoped model controls available for a global conversation', async () => {
    render(<SwitcherHarness workspace="" />)

    expect(await screen.findByRole('button', { name: '切换模型，当前：Turbo 中' })).toBeEnabled()
  })
})

function SwitcherHarness({ workspace = '/tmp/book' }: { workspace?: string }) {
  const [snapshot, setSnapshot] = useState<ConversationConfigSnapshot>({
    agent_kind: 'ide',
    profile_id: 'fast',
    thinking_level: 'medium',
    approval_mode: 'write',
    revision: 1,
  })
  return (
    <ModelProfileSwitcher
      agentKey="ide"
      workspace={workspace}
      conversationConfig={{
        snapshot,
        initialized: true,
        loading: false,
        saving: false,
        error: null,
        reload: async () => snapshot,
        patch: async (changes) => {
          patchConversationConfig(changes)
          setSnapshot((current) => ({ ...current, ...changes, revision: current.revision + 1 }))
          return true
        },
      }}
    />
  )
}

function settingsSnapshot(patch: Partial<LayeredSettings>): LayeredSettings {
  return {
    default: {},
    global: {},
    user: {},
    workspace: {},
    effective: {},
    paths: {
      denova_dir: '/denova',
      nova_dir: '/nova',
      user_config: '/nova/config.toml',
      workspace_config: '/tmp/book/.nova/config.toml',
    },
    builtin_agent_prompts: {},
    builtin_agent_prompt_blocks: {},
    builtin_agent_prompt_sources: {},
    resolved_agent_tool_manifests: {},
    resolved_agent_contexts: {},
    ...patch,
  }
}
