import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import type { ConversationConfigController } from '@/features/conversation-config/types'
import { ModelProfileSwitcher } from './ModelProfileSwitcher'
import { ToolNavigationProvider } from './tool-navigation'

const settingsMocks = vi.hoisted(() => ({
  fetchSettings: vi.fn(),
  fetchProjectSettings: vi.fn(),
  fetchEngineModels: vi.fn(),
  profiles: [] as { id: string; label: string; modelLabel: string }[],
}))

vi.mock('@/features/agent-runtime/api', () => ({ fetchEngineModels: settingsMocks.fetchEngineModels }))
vi.mock('@/features/agent-runtime/api-profiles', () => ({ useRuntimeProfiles: () => ({ profiles: settingsMocks.profiles, loaded: true, failed: false }) }))

vi.mock('@/features/settings/api', () => ({
  fetchSettings: settingsMocks.fetchSettings,
  fetchProjectSettings: settingsMocks.fetchProjectSettings,
}))

vi.mock('@/features/settings/query', () => ({
  GLOBAL_SETTINGS_TARGET: 'global',
  subscribeSettingsTarget: () => () => {},
}))

describe('ModelProfileSwitcher', () => {
  for (const agentKey of ['ide', 'general', 'interactive_story'] as const) {
    it(`links the ${agentKey} Native runtime to its Agents configuration`, async () => {
      settingsMocks.fetchSettings.mockResolvedValue({ effective: { openai_model: 'test-model' } })
      const open = vi.fn(() => ({ pointerEvents: document.body.style.pointerEvents, menu: screen.queryByRole('menu') }))
      const binding = { mode: agentKey === 'ide' ? 'writing' : agentKey === 'general' ? 'agent_chat' : 'interactive', project_id: 'project', session_id: 'session' } as const
      const controller: ConversationConfigController = {
        binding, snapshot: { agent_kind: agentKey, profile_id: 'default', thinking_level: 'medium', approval_mode: 'write', revision: 1 },
        initialized: true, loading: false, saving: false, error: null, patch: vi.fn(), reload: vi.fn(),
      }
      render(<ToolNavigationProvider value={{ workspace: '', open }}><ModelProfileSwitcher agentKey={agentKey} conversationConfig={controller} /></ToolNavigationProvider>)
      await waitFor(() => expect(screen.getByRole('button', { name: /切换模型/ })).toBeEnabled())
      await userEvent.click(screen.getByRole('button', { name: /切换模型/ }))
      expect(screen.queryByText('切换运行时')).not.toBeInTheDocument()
      await userEvent.click(screen.getByRole('menuitem', { name: '运行时：Native' }))
      await waitFor(() => expect(open).toHaveBeenCalledWith({ kind: 'config_resource', resource: 'agent_profile', id: agentKey, scope: 'user', section: 'runtime' }))
      // Navigation can mount a new overlay; the old modal must release its lock first.
      expect(open.mock.results[0].value).toEqual({ pointerEvents: '', menu: null })
      expect(open).toHaveBeenCalledTimes(1)
      await userEvent.click(screen.getByRole('button', { name: /切换模型/ }))
      await userEvent.keyboard('{Escape}')
      await waitFor(() => expect(screen.queryByRole('menu')).not.toBeInTheDocument())
      expect(screen.getByRole('button', { name: /切换模型/ })).toHaveFocus()
      expect(open).toHaveBeenCalledTimes(1)
    })
  }
  it('uses only Denova profiles without requesting CLI models for an API conversation', async () => {
    settingsMocks.fetchEngineModels.mockClear()
    settingsMocks.profiles = [{ id: 'profile:api', label: 'Gateway', modelLabel: 'Gateway' }, { id: 'profile:second', label: 'Second gateway', modelLabel: 'Second gateway' }]
    const patch = vi.fn().mockResolvedValue(true)
    const controller: ConversationConfigController = {
      snapshot: { agent_kind: 'ide', profile_id: 'default', thinking_level: 'medium', approval_mode: 'write', revision: 1,
        runtime: { kind: 'codex', codex: { profile_id: 'api', sandbox: 'read-only' } } },
      initialized: true, loading: false, saving: false, error: null, patch, reload: vi.fn(),
    }
    try {
      render(<ModelProfileSwitcher agentKey="ide" conversationConfig={controller} />)
      await userEvent.click(screen.getByRole('button', { name: /切换模型/ }))
      await userEvent.click(screen.getByRole('menuitem', { name: 'API · Second gateway' }))
      expect(patch).toHaveBeenCalledWith({ codex: { profile_id: 'second', sandbox: 'read-only' } })
      expect(settingsMocks.fetchEngineModels).not.toHaveBeenCalled()
    } finally { settingsMocks.profiles = [] }
  })
  it('shows the external model without fetching or editing dormant Native profiles', async () => {
    settingsMocks.fetchEngineModels.mockResolvedValue({ items: [] })
    settingsMocks.fetchSettings.mockClear()
    const patch = vi.fn()
    const controller: ConversationConfigController = {
      snapshot: { agent_kind: 'ide', profile_id: 'removed', thinking_level: 'medium', approval_mode: 'write', revision: 2,
        runtime: { kind: 'codex', codex: { model: 'external-model' } } },
      initialized: true, loading: false, saving: false, error: null, patch, reload: vi.fn(),
    }
    render(<ModelProfileSwitcher agentKey="ide" conversationConfig={controller} />)
    expect(await screen.findByText(/external-model/)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /切换模型/ })).toBeEnabled()
    expect(settingsMocks.fetchSettings).not.toHaveBeenCalled()
    expect(patch).not.toHaveBeenCalled()
  })
  for (const agentKey of ['ide', 'general'] as const) {
    it(`edits ${agentKey} Codex model and effort using the conversation controller`, async () => {
      settingsMocks.fetchEngineModels.mockResolvedValue({ items: [
        { id: 'external-model', display_name: 'External model', efforts: ['medium', 'high'] },
        { id: 'second-model', display_name: 'Second model', efforts: ['low'] },
      ] })
      const patch = vi.fn().mockResolvedValue(true)
      const controller: ConversationConfigController = {
        snapshot: { agent_kind: agentKey, profile_id: 'removed', thinking_level: 'medium', approval_mode: 'write', revision: 0,
          runtime: { kind: 'codex', codex: { model: 'external-model', effort: 'medium', sandbox: 'read-only' } } },
        initialized: true, loading: false, saving: false, error: null, patch, reload: vi.fn(),
      }
      const user = userEvent.setup()
      render(<ModelProfileSwitcher agentKey={agentKey} conversationConfig={controller} />)
      await user.click(screen.getByRole('button', { name: /切换模型/ }))
      await user.click(await screen.findByRole('button', { name: '高' }))
      expect(patch).toHaveBeenLastCalledWith({ codex: { model: 'external-model', effort: 'high', sandbox: 'read-only' } })
      await user.click(screen.getByRole('button', { name: /切换模型/ }))
      await user.click(await screen.findByRole('button', { name: '默认' }))
      expect(patch).toHaveBeenLastCalledWith({ codex: { model: 'external-model', sandbox: 'read-only' } })
      await user.click(screen.getByRole('button', { name: /切换模型/ }))
      await user.click(await screen.findByRole('menuitem', { name: 'Second model' }))
      expect(patch).toHaveBeenLastCalledWith({ codex: { model: 'second-model', sandbox: 'read-only' } })
    })
  }

  it('keeps Codex selection read-only while its execution is active', async () => {
    settingsMocks.fetchEngineModels.mockResolvedValue({ items: [{ id: 'external-model', display_name: 'External model', efforts: ['high'] }] })
    const patch = vi.fn()
    const controller: ConversationConfigController = {
      snapshot: { agent_kind: 'general', profile_id: 'default', thinking_level: 'medium', approval_mode: 'write', revision: 2,
        runtime: { kind: 'codex', codex: { model: 'external-model' } } },
      initialized: true, loading: false, saving: false, error: null, patch, reload: vi.fn(),
    }
    render(<ModelProfileSwitcher agentKey="general" conversationConfig={controller} runActive />)
    await userEvent.click(screen.getByRole('button', { name: /切换模型/ }))
    expect(screen.getByRole('note')).toHaveTextContent('请等待本次执行')
    await waitFor(() => expect(screen.getByRole('button', { name: '高' })).toBeDisabled())
    expect(patch).not.toHaveBeenCalled()
  })
  it('retains the saved selection when the Codex catalog is unavailable', async () => {
    const warning = vi.spyOn(console, 'warn').mockImplementation(() => {})
    settingsMocks.fetchEngineModels.mockRejectedValue(new Error('offline'))
    const controller: ConversationConfigController = {
      snapshot: { agent_kind: 'general', profile_id: 'default', thinking_level: 'medium', approval_mode: 'write', revision: 2,
        runtime: { kind: 'codex', codex: { model: 'saved-model', effort: 'high' } } },
      initialized: true, loading: false, saving: false, error: null, patch: vi.fn(), reload: vi.fn(),
    }
    try {
      render(<ModelProfileSwitcher agentKey="general" conversationConfig={controller} />)
      await userEvent.click(screen.getByRole('button', { name: /saved-model/ }))
      expect(await screen.findByText('连接引擎失败，请检查后重试。')).toBeInTheDocument()
      expect(controller.patch).not.toHaveBeenCalled()
      expect(screen.queryByRole('button', { name: '默认' })).not.toBeInTheDocument()
    } finally { warning.mockRestore() }
  })
  it('saves a next-turn model selection while a model turn is active', async () => {
    settingsMocks.fetchSettings.mockResolvedValue({
      effective: { openai_model: 'test-model' },
    })
    const user = userEvent.setup()
    const patch = vi.fn().mockResolvedValue(true)
    const controller: ConversationConfigController = {
      snapshot: {
        agent_kind: 'ide',
        profile_id: 'default',
        thinking_level: 'medium',
        approval_mode: 'write',
        revision: 1,
      },
      initialized: true,
      loading: false,
      saving: false,
      error: null,
      patch,
      reload: vi.fn().mockResolvedValue(null),
    }

    render(
      <ModelProfileSwitcher
        agentKey="ide"
        conversationConfig={controller}
        runActive
      />,
    )

    await user.click(await screen.findByRole('button', { name: /切换模型/ }))
    expect(screen.getByRole('note')).toHaveTextContent('已开始的模型轮次保持当前配置；修改从下一模型轮次生效。')
    const highThinking = screen.getByRole('button', { name: '高' })
    expect(highThinking).toBeEnabled()

    await user.click(highThinking)
    expect(patch).toHaveBeenCalledWith({ thinking_level: 'high' })
  })
})
