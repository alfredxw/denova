import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import type { ContextAnalysis, ContextAnalysisPart } from '@/lib/api'
import { ContextAnalysisDialog } from './ContextAnalysisDialog'

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
    bytes: input.bytes ?? content.length,
    chars: input.chars ?? content.length,
  }
}
