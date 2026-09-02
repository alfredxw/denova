import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { ToolExecutionBlock } from './message-tool'

describe('ToolExecutionBlock', () => {
  it('progressively formats incomplete JSON tool input without interpreting it as Markdown', async () => {
    const rawInput = '{"path":"chapters/ch01.md","options":{"recursive":tr'
    const { container } = render(<ToolExecutionBlock
      message={{
        id: 'streaming-write',
        role: 'tool_call',
        name: 'write',
        args: rawInput,
        status: 'running',
        streaming: true,
      }}
    />)

    const input = container.querySelector('[data-nova-tool-input-stream]')
    expect(input).toHaveAttribute('aria-busy', 'true')
    await waitFor(() => {
      expect(input).toHaveTextContent('"recursive": true')
    })
    expect(input?.textContent).toBe(`{
  "path": "chapters/ch01.md",
  "options": {
    "recursive": true
  }
}`)
    expect(input?.querySelector('a, em')).not.toBeInTheDocument()
  })

  it('keeps non-JSON streaming input visible as raw text', async () => {
    const rawInput = '*files* [chapter](https://example.com)'
    const { container } = render(<ToolExecutionBlock
      message={{
        id: 'streaming-raw',
        role: 'tool_call',
        name: 'unknown_tool',
        args: rawInput,
        status: 'running',
        streaming: true,
      }}
    />)

    const input = container.querySelector('[data-nova-tool-input-stream]')
    await waitFor(() => {
      expect(input).toHaveTextContent(rawInput)
    })
    expect(input?.textContent).toBe(rawInput)
    expect(input?.querySelector('a, em')).not.toBeInTheDocument()
  })

  it('removes the streaming projection when the tool input completes', async () => {
    const message = {
      id: 'completed-input',
      role: 'tool_call' as const,
      name: 'unknown_tool',
      args: '{"path":"chapters/ch01.md"',
      status: 'running' as const,
      streaming: true,
    }
    const { container, rerender } = render(<ToolExecutionBlock message={message} />)

    await waitFor(() => {
      expect(container.querySelector('[data-nova-tool-input-stream]')).toHaveTextContent('"path": "chapters/ch01.md"')
    })
    rerender(<ToolExecutionBlock message={{ ...message, args: '{"path":"chapters/ch01.md"}', streaming: false }} />)

    expect(container.querySelector('[data-nova-tool-input-stream]')).not.toBeInTheDocument()
  })

  it('presents a bounded directory read as an expected partial result', () => {
    const metadata = JSON.stringify({
      schema: 'resource.read.v1',
      status: 'partial',
      source: { kind: 'directory', path: '.' },
      limits: { returned: 200, truncated: true },
      recovery: { retryable: true, suggestion: 'Narrow the resource path or requested limit.' },
    })

    const { container } = render(<ToolExecutionBlock
      message={{
        role: 'tool_call',
        name: 'read',
        args: JSON.stringify({ path: '.', limit: 200, depth: 2, hidden: false }),
        result: `${metadata}\nAGENTS.md\nCHANGELOG.md`,
        status: 'success',
      }}
    />)

    expect(screen.getByText(/已返回当前批次/)).toBeInTheDocument()
    expect(screen.queryByText(/结果不完整/)).not.toBeInTheDocument()
    expect(screen.queryByText(/Narrow the resource path/)).not.toBeInTheDocument()
    expect(container.querySelector('.lucide-triangle-alert')).not.toBeInTheDocument()
  })

  it('shows both offsets required to continue a truncated line', () => {
    const metadata = JSON.stringify({
      schema: 'resource.read.v1',
      status: 'partial',
      source: { kind: 'file', path: 'long.txt' },
      limits: { truncated: true, next_offset: 1, next_byte_offset: 65536 },
    })

    render(<ToolExecutionBlock
      message={{
        role: 'tool_call',
        name: 'read',
        args: JSON.stringify({ path: 'long.txt' }),
        result: `${metadata}\nlong line fragment`,
        status: 'success',
      }}
    />)

    expect(screen.getByText(/offset=1、byte_offset=65536/)).toBeInTheDocument()
  })

  it('presents a rule check as one complete creator-facing audit', () => {
    const { container } = render(<ToolExecutionBlock
      message={{
        role: 'tool_call',
        name: 'prepare_interactive_turn',
        args: JSON.stringify({
          action: '穿过失控的蒸汽阀门',
          intent: '在巡逻队赶到前进入控制室',
          challenge: '阀门喷出的高温蒸汽封住了唯一通路',
          cost: '失败会损失体力，并惊动正在接近的巡逻队',
          state: '主角体力尚可，但只携带一件隔热披风',
          adjudication: {
            reason: '行动存在明确风险，失败会改变后续局势。',
            stakes: '受伤并暴露潜入行动。',
            difficulty_reason: '通路狭窄且蒸汽持续喷发，因此采用困难难度。',
            roll_mode_reason: '隔热披风提供帮助，但不足以形成优势。',
            state_refs: [{ actor_id: 'protagonist', field_id: '体力' }],
          },
          rule: {
            template: 'dice_check',
            template_id: 'balanced-check',
            label: '均衡骰子检定',
            failure_policy: 'fail_forward',
            roll_mode: 'normal',
          },
          bonuses: [
            { kind: 'equipment', reason: '隔热披风', value: 2 },
            { kind: 'environment', reason: '视野受阻', value: -1 },
          ],
          difficulty: 'hard',
          outcomes: {
            critical_success: { result: '毫发无伤地穿过，并关闭阀门。' },
            success: { result: '穿过蒸汽，但披风被烧毁。' },
            failure: { result: '被迫后退，巡逻队进一步逼近。' },
            critical_failure: { result: '严重灼伤，并触发控制室警报。' },
          },
        }),
        result: JSON.stringify({
          resolution_id: 'resolution-1',
          label: '均衡骰子检定',
          dice: '1d20',
          roll_mode: 'normal',
          rolls: [1],
          kept_roll: 1,
          bonus_total: 15,
          bonus_details: [
            { kind: 'equipment', reason: '隔热披风', value: 2 },
            { kind: 'state', actor_id: 'protagonist', field_id: '体力', reason: '当前体力', value: 3 },
            { kind: 'story', reason: 'Story-wide roll modifier.', value: 10 },
          ],
          base_target: 20,
          total: 16,
          difficulty: 'very_hard',
          requested_difficulty: 'hard',
          difficulty_shift: 1,
          target: 25,
          outcome: 'critical_failure',
          result: '严重灼伤，并触发控制室警报。',
          cost: '失败会损失体力，并惊动正在接近的巡逻队',
          stakes: '受伤并暴露潜入行动。',
          state_changes: [{ actor_id: 'protagonist', field_id: '体力', change: -3, reason: '蒸汽灼伤' }],
        }),
        status: 'success',
      }}
    />)

    expect(container.querySelector('[data-nova-tool-summary]')).toHaveTextContent('大失败 · 严重灼伤，并触发控制室警报。')
    fireEvent.click(container.querySelector('[data-nova-tool-header]') as HTMLElement)

    expect(container.querySelector('[data-nova-tool-detail-unified]')).toBeInTheDocument()
    expect(container.querySelector('[data-nova-tool-detail-input]')).not.toBeInTheDocument()
    expect(container.querySelector('[data-nova-tool-detail-output]')).not.toBeInTheDocument()
    expect(screen.getByText('在巡逻队赶到前进入控制室')).toBeInTheDocument()
    expect(screen.getByText('行动存在明确风险，失败会改变后续局势。')).toBeInTheDocument()
    expect(screen.getAllByText('protagonist.体力').length).toBeGreaterThan(0)
    expect(screen.getByText('毫发无伤地穿过，并关闭阀门。')).toBeInTheDocument()
    expect(screen.getByText('穿过蒸汽，但披风被烧毁。')).toBeInTheDocument()
    expect(screen.getByText('被迫后退，巡逻队进一步逼近。')).toBeInTheDocument()
    expect(screen.getAllByText('严重灼伤，并触发控制室警报。')).toHaveLength(2)
    expect(screen.getByText(/1d20 \[1\].*\+15.*16 \/ 25/)).toBeInTheDocument()
    expect(screen.getByText('全局检定修正')).toBeInTheDocument()
    expect(screen.queryByText('Story-wide roll modifier.')).not.toBeInTheDocument()
  })

  it('keeps every submitted turn module in one scrollable detail view', () => {
    const stateChanges = Array.from({ length: 14 }, (_, index) => ({
      op: index % 2 === 0 ? 'replace' : 'delta',
      actor_id: `actor-${index + 1}`,
      field_id: `field-${index + 1}`,
      value: index % 2 === 0 ? `value-${index + 1}` : index + 1,
    }))
    const choices = ['继续追踪巡逻队', '关闭蒸汽总阀', '搜查控制室', '联络外部接应', '原路撤退']
    const { container } = render(<ToolExecutionBlock
      message={{
        role: 'tool_call',
        name: 'submit_interactive_turn',
        args: JSON.stringify({
          state_changes: stateChanges,
          choices,
          plan_update: {
            mode: 'replace_sections',
            sections: [
              { heading: '## 下一幕', markdown: '跟进控制室警报与巡逻队的反应。' },
              { heading: '## 未解决线索', markdown: '确认蒸汽事故是否由内应制造。' },
            ],
          },
        }),
        result: JSON.stringify({
          ready: true,
          module_status: {
            state_changes: 'accepted',
            choices: 'accepted',
            plan_update: 'accepted',
          },
        }),
        status: 'success',
      }}
    />)

    expect(container.querySelector('[data-nova-tool-summary]')).toHaveTextContent('回合已提交 · 14 项状态变化 · 5 个行动选项 · 规划已更新')
    fireEvent.click(container.querySelector('[data-nova-tool-header]') as HTMLElement)

    expect(container.querySelector('[data-nova-tool-detail-scroll]')).toHaveClass('max-h-[min(30dvh,18rem)]')
    expect(screen.getByText(/actor-14\.field-14/)).toBeInTheDocument()
    expect(screen.getByText('原路撤退')).toBeInTheDocument()
    expect(screen.getByText('## 下一幕')).toBeInTheDocument()
    expect(screen.getByText('跟进控制室警报与巡逻队的反应。')).toBeInTheDocument()
    expect(screen.getByText('## 未解决线索')).toBeInTheDocument()
    expect(screen.getByText('确认蒸汽事故是否由内应制造。')).toBeInTheDocument()
    expect(screen.queryByText(/state_changes=accepted/)).not.toBeInTheDocument()
  })

  it('localizes partial turn submission diagnostics and preserves retry details', () => {
    const { container } = render(<ToolExecutionBlock
      message={{
        role: 'tool_call',
        name: 'submit_interactive_turn',
        args: JSON.stringify({ choices: ['留在原地', '留在原地'] }),
        result: JSON.stringify({
          ready: false,
          module_status: { state_changes: 'accepted', choices: 'rejected', plan_update: 'missing' },
          diagnostics: [{
            module: 'choices',
            code: 'duplicate_choice',
            severity: 'error',
            path: '/choices/1',
            retryable: true,
            message: 'Choices must remain distinct after text normalization.',
          }],
          retry_modules: ['choices'],
          missing_modules: ['plan_update'],
          diagnostics_truncated: true,
          plan_update_detail: {
            mode: 'replace_sections',
            accepted_sections: ['## 下一幕'],
            retry_sections: ['## 未解决线索'],
            retained_draft: true,
          },
        }),
        status: 'success',
      }}
    />)

    expect(container.querySelector('[data-nova-tool-summary]')).toHaveTextContent('回合尚未就绪 · 2 个行动选项')
    fireEvent.click(container.querySelector('[data-nova-tool-header]') as HTMLElement)

    expect(screen.getByText('提交内容需要处理')).toBeInTheDocument()
    expect(screen.getByText('行动选项存在重复。')).toBeInTheDocument()
    expect(screen.getByText('/choices/1')).toBeInTheDocument()
    expect(screen.getAllByText('分支规划').length).toBeGreaterThan(0)
    expect(screen.getByText('需重试段落')).toBeInTheDocument()
    expect(screen.getByText('## 未解决线索')).toBeInTheDocument()
    expect(screen.getByText('问题较多，这里只展示了其中一部分。')).toBeInTheDocument()
    expect(screen.queryByText('Choices must remain distinct after text normalization.')).not.toBeInTheDocument()
  })
})
