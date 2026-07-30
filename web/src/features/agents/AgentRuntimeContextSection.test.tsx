import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import type { ResolvedAgentContextSettings } from '@/features/settings/types'
import { AgentRuntimeContextSection } from './AgentRuntimeContextSection'

const resolvedContext: ResolvedAgentContextSettings = {
  compaction_enabled: true,
  compaction_threshold: 0.85,
  tool_result_context_enabled: true,
  max_fragment_bytes: 256 * 1024,
  max_total_injected_bytes: 1024 * 1024,
  max_fragments: 256,
  max_metadata_field_bytes: 4 * 1024,
  max_provider_input_bytes: 4 * 1024 * 1024,
}

describe('AgentRuntimeContextSection', () => {
  it('renders backend-resolved values and emits only the edited intent', () => {
    const onChange = vi.fn()
    render(
      <AgentRuntimeContextSection
        agent="ide"
        value={{}}
        resolved={resolvedContext}
        onChange={onChange}
      />,
    )

    expect(screen.getByLabelText('触发阈值 (%)')).toHaveValue(85)
    expect(screen.getByLabelText('本轮片段数量上限')).toHaveValue(256)
    expect(screen.getByRole('switch', { name: '工具结果上下文' })).toBeChecked()

    fireEvent.change(screen.getByLabelText('触发阈值 (%)'), { target: { value: '65' } })
    expect(onChange).toHaveBeenLastCalledWith({ compaction_threshold: 0.65 })
  })

  it('uses the local draft while waiting for the normalized server snapshot', () => {
    render(
      <AgentRuntimeContextSection
        agent="ide"
        value={{ compaction_threshold: 0.72, max_fragments: 64 }}
        resolved={resolvedContext}
        onChange={vi.fn()}
      />,
    )

    expect(screen.getByLabelText('触发阈值 (%)')).toHaveValue(72)
    expect(screen.getByLabelText('本轮片段数量上限')).toHaveValue(64)
  })

  it('keeps backend-managed compactor mechanics out of the editor', () => {
    render(
      <AgentRuntimeContextSection
        agent="context_compaction"
        value={{}}
        resolved={{ ...resolvedContext, tool_result_context_enabled: false }}
        onChange={vi.fn()}
      />,
    )

    expect(screen.queryByLabelText('触发阈值 (%)')).not.toBeInTheDocument()
    expect(screen.queryByRole('switch', { name: '工具结果上下文' })).not.toBeInTheDocument()
    expect(screen.getByLabelText('单片段上限 (KB)')).toHaveValue(256)
  })
})
