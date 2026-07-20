import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { ContextAnalysis, ContextAnalysisPart } from '@/lib/api'
import { ContextAnalysisDialog } from './ContextAnalysisDialog'

const originalClipboardDescriptor = Object.getOwnPropertyDescriptor(navigator, 'clipboard')

describe('ContextAnalysisDialog', () => {
  it('renders a single-part final message group without a duplicate nested card', async () => {
    render(
      <ContextAnalysisDialog
        open
        loading={false}
        error={null}
        analysis={analysisFixture([
          partFixture({
            id: 'world_context',
            source: '世界上下文',
            title: '世界上下文',
            role: 'user',
            kind: 'body',
            note: 'final_user_message',
            content: '青云山: 节点名称=青云山',
          }),
        ])}
        onOpenChange={() => {}}
      />,
    )

    expect(screen.getAllByText('#1 世界上下文')).toHaveLength(1)
    expect(screen.queryByText('青云山: 节点名称=青云山')).not.toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: /#1 世界上下文/ }))

    expect(screen.getByText('青云山: 节点名称=青云山')).toBeInTheDocument()
  })

  it('expands inner parts by default when a multi-part message group is opened', async () => {
    render(
      <ContextAnalysisDialog
        open
        loading={false}
        error={null}
        analysis={analysisFixture([
          partFixture({
            id: 'turn_user',
            source: '互动历史回合',
            title: '历史回合消息 28',
            role: 'user',
            kind: 'body',
            content: '我要前进',
          }),
          partFixture({
            id: 'turn_assistant',
            source: '互动历史回合',
            title: '历史回合消息 29',
            role: 'assistant',
            kind: 'body',
            content: '助手回应',
          }),
        ])}
        onOpenChange={() => {}}
      />,
    )

    expect(screen.queryByText('我要前进')).not.toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: /对话回合 #1/ }))

    expect(screen.getByText('我要前进')).toBeInTheDocument()
    expect(screen.getByText('助手回应')).toBeInTheDocument()
  })

  it('copies the full model context and the exact raw content of one message group', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', {
      configurable: true,
      value: { writeText },
    })
    const currentTurnContent = '[互动输入]\n用户本回合行动：\n我推开门'
    render(
      <ContextAnalysisDialog
        open
        loading={false}
        error={null}
        analysis={analysisFixture([
          partFixture({
            id: 'current_turn',
            source: '本轮互动指令',
            title: '本轮互动指令与动态上下文',
            role: 'user',
            kind: 'body',
            content: currentTurnContent,
            parts: [
              partFixture({ source: '本轮行动', title: '当前用户行动', content: '我推开门' }),
              partFixture({ source: 'agent-brief.md', title: '正文 Agent 简报', content: '守住当前目标' }),
            ],
          }),
        ])}
        onOpenChange={() => {}}
      />,
    )

    await userEvent.click(screen.getByRole('button', { name: '复制全部上下文' }))
    expect(writeText).toHaveBeenLastCalledWith(expect.stringContaining('[system]\nsystem'))
    expect(writeText).toHaveBeenLastCalledWith(expect.stringContaining(`[user]\n${currentTurnContent}`))

    await userEvent.click(screen.getByRole('button', { name: '复制本组' }))
    expect(writeText).toHaveBeenLastCalledWith(currentTurnContent)

    await userEvent.click(screen.getByRole('button', { name: /本轮对话/ }))
    expect(screen.getByText('正文 Agent 简报')).toBeInTheDocument()
    const copyPartButtons = screen.getAllByRole('button', { name: '复制片段' })
    await userEvent.click(copyPartButtons[copyPartButtons.length - 1])
    expect(writeText).toHaveBeenLastCalledWith('守住当前目标')
  })
})

afterEach(() => {
  vi.restoreAllMocks()
  if (originalClipboardDescriptor) Object.defineProperty(navigator, 'clipboard', originalClipboardDescriptor)
  else Reflect.deleteProperty(navigator, 'clipboard')
})

function analysisFixture(contextMessages: ContextAnalysisPart[]): ContextAnalysis {
  return {
    agent_kind: 'interactive',
    mode: 'interactive',
    system_prompt: 'system',
    system_prompt_parts: [
      partFixture({
        id: 'system',
        source: 'SystemPrompt',
        title: 'SystemPrompt',
        content: '系统提示',
      }),
    ],
    context_parts: contextMessages,
    context_messages: contextMessages,
    message_count: contextMessages.length,
    token_estimate: 120,
    context_window_tokens: 128000,
    context_usage_ratio: 0.01,
    compaction_active: false,
    would_compact: false,
  }
}

function partFixture(input: Partial<ContextAnalysisPart>): ContextAnalysisPart {
  const content = input.content || ''
  return {
    id: input.id || '',
    source: input.source || '',
    title: input.title || '',
    role: input.role || '',
    kind: input.kind || '',
    tool_name: input.tool_name || '',
    tool_call_id: input.tool_call_id || '',
    content,
    note: input.note || '',
    parts: input.parts,
    bytes: input.bytes ?? content.length,
    chars: input.chars ?? content.length,
  }
}
