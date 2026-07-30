import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { AgentRuntimeContextSection, normalizeContextPressurePatch } from './AgentRuntimeContextSection'

describe('AgentRuntimeContextSection', () => {
  it('renders inherited pressure defaults and emits typed overrides', () => {
    const onChange = vi.fn()
    render(
      <AgentRuntimeContextSection
        agent="ide"
        value={{}}
        inherited={{
          compaction_threshold: 0.85,
          tool_result_cleanup_threshold: 0.7,
          tool_result_cleanup_target: 0.6,
          tool_result_keep_recent: 3,
          compaction_recovery_band: 0.8,
          compaction_max_consecutive_failures: 3,
        }}
        onChange={onChange}
      />,
    )

    expect(screen.getByLabelText('工具结果清理阈值 (%)')).toHaveValue(70)
    expect(screen.getByLabelText('工具结果清理目标 (%)')).toHaveValue(60)
    expect(screen.getByLabelText('保护最近工具交互组')).toHaveValue(3)
    expect(screen.getByLabelText('压缩恢复系数 (%)')).toHaveValue(80)

    fireEvent.change(screen.getByLabelText('最大连续失败次数'), { target: { value: '5' } })
    expect(onChange).toHaveBeenLastCalledWith({ compaction_max_consecutive_failures: 5 })
  })

  it('keeps compactor-only output controls separate from runtime cleanup policy', () => {
    render(
      <AgentRuntimeContextSection
        agent="context_compaction"
        value={{}}
        inherited={{}}
        onChange={vi.fn()}
      />,
    )

    expect(screen.getByLabelText('压缩后保留回合')).toBeInTheDocument()
    expect(screen.queryByLabelText('工具结果清理阈值 (%)')).not.toBeInTheDocument()
  })

  it('keeps cleanup and compaction ratios coherent while editing', () => {
    const onChange = vi.fn()
    render(
      <AgentRuntimeContextSection
        agent="ide"
        value={{}}
        inherited={{
          compaction_threshold: 0.85,
          tool_result_cleanup_threshold: 0.7,
          tool_result_cleanup_target: 0.6,
        }}
        onChange={onChange}
      />,
    )

    fireEvent.change(screen.getByLabelText('触发阈值 (%)'), { target: { value: '65' } })
    expect(onChange).toHaveBeenLastCalledWith({
      compaction_threshold: 0.65,
      tool_result_cleanup_threshold: 0.65 * 0.85,
      tool_result_cleanup_target: 0.65 * 0.85 * 0.85,
    })
  })

  it('keeps inherited pressure ratios coherent when an override is reset', () => {
    expect(normalizeContextPressurePatch(
      {
        compaction_threshold: 0.9,
        tool_result_cleanup_threshold: 0.8,
        tool_result_cleanup_target: 0.7,
      },
      {
        compaction_threshold: 0.65,
        tool_result_cleanup_threshold: 0.55,
        tool_result_cleanup_target: 0.45,
      },
      { compaction_threshold: null },
    )).toEqual({
      compaction_threshold: null,
      tool_result_cleanup_threshold: 0.65 * 0.85,
      tool_result_cleanup_target: 0.65 * 0.85 * 0.85,
    })
  })
})
