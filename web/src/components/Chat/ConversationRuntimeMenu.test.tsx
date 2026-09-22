import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { DropdownMenu, DropdownMenuContent } from '@/components/ui/dropdown-menu'
import type { ConversationConfigController } from '@/features/conversation-config/types'
import { ConversationRuntimeMenu } from './ConversationRuntimeMenu'

const mocks = vi.hoisted(() => ({ fetchProjectSettings: vi.fn(), fetchSettings: vi.fn(), fetchModelCatalog: vi.fn(), fetchAgentEngines: vi.fn(), fetchEngineModels: vi.fn(), checkAgentEngine: vi.fn() }))
vi.mock('@/features/settings/api', () => mocks)
vi.mock('@/features/agent-runtime/api', () => mocks)

let user: ReturnType<typeof userEvent.setup>

function controller(agent: 'ide' | 'general' | 'interactive_story' = 'general'): ConversationConfigController {
  return {
    binding: { mode: agent === 'ide' ? 'writing' : agent === 'general' ? 'agent_chat' : 'interactive', project_id: 'project', session_id: 'session', story_id: 'story', branch_id: 'main' },
    snapshot: { agent_kind: agent, profile_id: 'default', thinking_level: 'medium', approval_mode: 'write', revision: 7 },
    initialized: true, loading: false, saving: false, error: null, patch: vi.fn().mockResolvedValue(true), reload: vi.fn(),
  }
}
function menu(config: ConversationConfigController, runActive = false) {
  return <DropdownMenu defaultOpen><DropdownMenuContent>
    <ConversationRuntimeMenu controller={config} runActive={runActive} onConfigure={vi.fn()} />
  </DropdownMenuContent></DropdownMenu>
}
async function openRuntime() {
  await user.click(screen.getByRole('menuitem', { name: '运行时：Native' }))
  await waitFor(() => expect(screen.queryByText('正在检查可用性…')).not.toBeInTheDocument())
}

beforeEach(() => {
  vi.clearAllMocks()
  // jsdom has no submenu geometry for Radix's pointer-grace handling. Exercise
  // clicks here; the browser suite covers pointer travel, hover, and placement.
  user = userEvent.setup({ skipHover: true })
  mocks.fetchProjectSettings.mockResolvedValue({ effective: { openai_model: 'native-model' } })
  mocks.fetchAgentEngines.mockResolvedValue({ items: ['native', 'codex', 'claude'].map(id => ({ id, status: 'ready' })) })
  mocks.fetchEngineModels.mockResolvedValue({ default_id: 'saved-model', items: [{ id: 'saved-model', efforts: ['high'] }] })
})

describe('ConversationRuntimeMenu', () => {
  for (const agent of ['ide', 'general', 'interactive_story'] as const) {
    it(`switches the bound ${agent} conversation and keeps the model menu open`, async () => {
      mocks.fetchProjectSettings.mockResolvedValue({ effective: { openai_model: 'native-model', agent_runtimes: { [agent]: { codex: { model: 'saved-model', effort: 'high' } } } } })
      const config = controller(agent)
      render(menu(config))
      await openRuntime()
      await user.click(await screen.findByRole('menuitem', { name: 'Codex' }))
      await waitFor(() => expect(config.patch).toHaveBeenCalledWith({ runtime: { kind: 'codex', codex: { model: 'saved-model', effort: 'high' } } }))
      await waitFor(() => expect(screen.getAllByRole('menu')).toHaveLength(1))
      expect(mocks.fetchProjectSettings).toHaveBeenCalledWith('project')
    })
  }
  it('disables unavailable runtimes and leaves configuration accessible', async () => {
    mocks.fetchAgentEngines.mockResolvedValue({ items: [{ id: 'codex', status: 'not_installed', reason_key: 'agentRuntime.notInstalled' }, { id: 'claude', status: 'auth_required' }] })
    const config = controller()
    render(menu(config))
    await openRuntime()
    expect(screen.getByRole('menuitem', { name: 'Codex' })).toHaveAttribute('aria-disabled', 'true')
    expect(screen.getByRole('menuitem', { name: 'Claude Code' })).toHaveAttribute('aria-disabled', 'true')
    expect(screen.getByText('需要登录')).toBeVisible()
    expect(screen.getAllByRole('menuitem', { name: '配置' }).every(item => item.getAttribute('aria-disabled') !== 'true')).toBe(true)
    expect(config.patch).not.toHaveBeenCalled()
  })
  it('checks an unknown runtime without automatically switching', async () => {
    mocks.fetchAgentEngines.mockResolvedValueOnce({ items: [{ id: 'codex', status: 'unchecked' }] })
    mocks.checkAgentEngine.mockResolvedValue({ id: 'codex', status: 'ready' })
    const config = controller()
    render(menu(config))
    await openRuntime()
    await user.click(screen.getByRole('menuitem', { name: 'Codex' }))
    await waitFor(() => expect(mocks.checkAgentEngine).toHaveBeenCalledWith('codex'))
    await waitFor(() => expect(screen.queryByText('正在检查可用性…')).not.toBeInTheDocument())
    expect(config.patch).not.toHaveBeenCalled()
    await user.click(screen.getByRole('menuitem', { name: 'Codex' }))
    await waitFor(() => expect(config.patch).toHaveBeenCalledOnce())
  })
  it('accepts a compatible API profile without CLI authentication', async () => {
    mocks.fetchProjectSettings.mockResolvedValue({ effective: {
      agent_runtimes: { general: { codex: { profile_id: 'api', sandbox: 'read-only' } } },
      model_profiles: [{ id: 'api', model: 'api-model', endpoint_id: 'endpoint' }],
      model_endpoints: [{ id: 'endpoint', protocol: 'openai-responses' }],
    } })
    mocks.fetchModelCatalog.mockResolvedValue({ providers: [], protocols: [] })
    mocks.fetchAgentEngines.mockResolvedValue({ items: [{ id: 'codex', status: 'auth_required' }] })
    const config = controller()
    render(menu(config))
    await openRuntime()
    await user.click(screen.getByRole('menuitem', { name: 'Codex' }))
    await waitFor(() => expect(config.patch).toHaveBeenCalledWith({ runtime: { kind: 'codex', codex: { profile_id: 'api', sandbox: 'read-only' } } }))
    expect(mocks.fetchEngineModels).not.toHaveBeenCalled()
  })
  it('rejects an incompatible API profile even when the engine is ready', async () => {
    mocks.fetchProjectSettings.mockResolvedValue({ effective: {
      agent_runtimes: { general: { codex: { profile_id: 'api' } } },
      model_profiles: [{ id: 'api', model: 'api-model', endpoint_id: 'endpoint' }],
      model_endpoints: [{ id: 'endpoint', protocol: 'anthropic-messages' }],
    } })
    mocks.fetchModelCatalog.mockResolvedValue({ providers: [], protocols: [] })
    render(menu(controller()))
    await openRuntime()
    expect(screen.getByRole('menuitem', { name: 'Codex' })).toHaveAttribute('aria-disabled', 'true')
  })
  it('rejects missing saved models instead of silently choosing another', async () => {
    mocks.fetchProjectSettings.mockResolvedValue({ effective: { agent_runtimes: { general: { codex: { model: 'removed' } } } } })
    render(menu(controller()))
    await openRuntime()
    expect(screen.getByRole('menuitem', { name: 'Codex' })).toHaveAttribute('aria-disabled', 'true')
  })
  it('preserves the session when availability changes before submission', async () => {
    const config = controller()
    render(menu(config))
    await openRuntime()
    mocks.fetchEngineModels.mockResolvedValue({ items: [] })
    await user.click(screen.getByRole('menuitem', { name: 'Codex' }))
    await waitFor(() => expect(screen.getByRole('menuitem', { name: 'Codex' })).toHaveAttribute('aria-disabled', 'true'))
    expect(config.patch).not.toHaveBeenCalled()
  })
  for (const state of ['running', 'draft', 'failure'] as const) {
    it(`preserves the runtime for ${state}`, async () => {
      const config = controller('interactive_story')
      if (state === 'draft') config.binding = { ...config.binding!, story_id: '' }
      if (state === 'failure') config.patch = vi.fn().mockResolvedValue(false)
      render(menu(config, state === 'running'))
      await openRuntime()
      const option = screen.getByRole('menuitem', { name: 'Codex' })
      if (state === 'failure') {
        await user.click(option)
        await waitFor(() => expect(config.patch).toHaveBeenCalledOnce())
        expect(screen.getAllByRole('menu')).toHaveLength(2)
      } else {
        expect(option).toHaveAttribute('aria-disabled', 'true')
        expect(config.patch).not.toHaveBeenCalled()
      }
      expect(config.snapshot?.runtime).toBeUndefined()
    })
  }
})
