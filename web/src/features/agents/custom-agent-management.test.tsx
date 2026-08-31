import type { ReactNode } from 'react'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import type { AgentRuntimeKind } from '@/features/settings/types'
import { AgentList, CreateCustomAgentDialog } from './custom-agent-management'

function renderWithTooltips(ui: ReactNode) {
  return render(<TooltipProvider delayDuration={0}>{ui}</TooltipProvider>)
}

describe('AgentList', () => {
  it('places the top-level create action before navigation and offers supported category shortcuts', async () => {
    const user = userEvent.setup()
    const onCreate = vi.fn<(runtimeKind: AgentRuntimeKind) => void>()
    renderWithTooltips(
      <AgentList active="ide" customAgents={[]} onSelect={vi.fn()} onCreate={onCreate} />,
    )

    const createButton = screen.getByRole('button', { name: '新建自定义 Agent' })
    const navigation = screen.getByRole('navigation')
    expect(createButton.compareDocumentPosition(navigation) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()

    const shortcuts: Array<[string, AgentRuntimeKind]> = [
      ['为General Agent新建自定义 Agent', 'general'],
      ['为写作 Agent新建自定义 Agent', 'ide'],
      ['为游戏 Agent新建自定义 Agent', 'interactive_story'],
      ['为图像 Agent新建自定义 Agent', 'image'],
    ]
    for (const [name, runtimeKind] of shortcuts) {
      await user.click(screen.getByRole('button', { name }))
      expect(onCreate).toHaveBeenLastCalledWith(runtimeKind)
    }
    expect(screen.queryByRole('button', { name: /为版本说明 Agent新建自定义 Agent/ })).not.toBeInTheDocument()
  })
})

describe('CreateCustomAgentDialog', () => {
  it('opens with the base selected by a category shortcut', () => {
    renderWithTooltips(
      <CreateCustomAgentDialog
        open
        initialRuntimeKind="interactive_story"
        onOpenChange={vi.fn()}
        onCreate={vi.fn()}
      />,
    )

    expect(screen.getByRole('combobox', { name: '运行契约' })).toHaveTextContent('游戏 Agent')
  })
})
