import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { DropdownMenu, DropdownMenuContent, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import type { AgentQuickPromptSettings } from '@/features/settings/types'
import { useAgentQuickPromptControls } from './use-agent-quick-prompt-controls'

const settings = vi.hoisted(() => ({
  customized: true,
  loading: false,
  showInCommands: false,
  prompts: [
    { id: 'fill', name: '填充操作', prompt: 'Fill this prompt', behavior: 'fill', enabled: true },
    { id: 'send', name: '发送操作', prompt: 'Send this prompt', behavior: 'send', enabled: true },
    { id: 'hidden', name: '隐藏操作', prompt: 'Hidden prompt', behavior: 'fill', enabled: false },
  ] as AgentQuickPromptSettings[],
  save: vi.fn(),
}))

vi.mock('./use-agent-quick-prompts', () => ({ useAgentQuickPrompts: () => settings }))

function Harness({ showCards = true, onFill = vi.fn(), onSend = vi.fn() }: {
  showCards?: boolean
  onFill?: (prompt: string) => void
  onSend?: (prompt: string) => void
}) {
  const controls = useAgentQuickPromptControls({ scope: 'skills', disabled: false, onFill, onSend })
  return <>
    {showCards ? controls.cards : <p>Existing conversation</p>}
    <DropdownMenu>
      <DropdownMenuTrigger>Composer settings</DropdownMenuTrigger>
      <DropdownMenuContent>{controls.menuItem}</DropdownMenuContent>
    </DropdownMenu>
    {controls.dialog}
  </>
}

describe('AgentQuickPrompts', () => {
  beforeEach(() => {
    settings.customized = true
    settings.showInCommands = false
    settings.save.mockReset().mockResolvedValue(undefined)
  })

  it('fills or sends cards according to each prompt setting', async () => {
    const user = userEvent.setup()
    const onFill = vi.fn()
    const onSend = vi.fn()
    render(<Harness onFill={onFill} onSend={onSend} />)

    await user.click(screen.getByRole('button', { name: /填充操作/ }))
    expect(onFill).toHaveBeenCalledWith('Fill this prompt')
    expect(onSend).not.toHaveBeenCalled()
    await user.click(screen.getByRole('button', { name: /发送操作/ }))
    expect(onSend).toHaveBeenCalledWith('Send this prompt')
    expect(screen.queryByText('隐藏操作')).not.toBeInTheDocument()
  })

  it('opens the same settings in an existing conversation and saves only the command preference', async () => {
    settings.customized = false
    const user = userEvent.setup()
    render(<Harness showCards={false} />)
    await user.click(screen.getByRole('button', { name: 'Composer settings' }))
    await user.click(screen.getByRole('menuitem', { name: '管理Skills快捷指令' }))
    expect(screen.getByRole('dialog')).toHaveTextContent('快捷指令 · Skills')
    expect(screen.getByDisplayValue('Fill this prompt')).toBeInTheDocument()
    await user.click(screen.getByRole('switch', { name: '在 / 菜单中显示快捷指令' }))
    await user.click(screen.getByRole('button', { name: '保存' }))
    expect(settings.save).toHaveBeenCalledWith({ showInCommands: true })
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })

  it('restores defaults without changing the independently saved command preference', async () => {
    settings.showInCommands = true
    const user = userEvent.setup()
    render(<Harness />)
    await user.click(screen.getByRole('button', { name: '管理Skills快捷指令' }))
    await user.click(screen.getByRole('button', { name: '恢复默认' }))
    await user.click(screen.getByRole('button', { name: '保存' }))
    expect(settings.save).toHaveBeenCalledWith({ prompts: null })
  })

  it('keeps an unsaved toggle local when settings are cancelled', async () => {
    const user = userEvent.setup()
    render(<Harness />)
    await user.click(screen.getByRole('button', { name: '管理Skills快捷指令' }))
    await user.click(screen.getByRole('switch', { name: '在 / 菜单中显示快捷指令' }))
    await user.click(screen.getByRole('button', { name: '取消' }))
    await user.click(screen.getByRole('button', { name: '管理Skills快捷指令' }))
    expect(screen.getByRole('switch', { name: '在 / 菜单中显示快捷指令' })).not.toBeChecked()
    expect(settings.save).not.toHaveBeenCalled()
  })
})
