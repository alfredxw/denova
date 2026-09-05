import { act, fireEvent, render } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'
import { setConfiguredLocale } from '@/i18n'
import { ToolExecutionBlock } from './message-tool'

const edits = [
  { old_string: 'existing', new_string: 'updated' },
  { old_string: 'same', new_string: 'unique' },
  { old_string: 'missing', new_string: 'found' },
]
const issues = [
  { edit_index: 1, code: 'not_unique', details: { match_count_at_least: 2 } },
  { edit_index: 2, code: 'not_found' },
]

function errorResult(items: unknown[] = issues) {
  return '[tool error]\n' + JSON.stringify({
    schema: 'workspace_change.tool_error.v1', code: 'invalid_edit',
    workspace_mutated: false,
    details: { path: 'state.md', issues: items, issue_count: items.length },
  })
}

afterEach(() => act(() => setConfiguredLocale('zh-CN')))

describe('edit failure diagnostics', () => {
  it.each([
    { locale: 'zh-CN', summary: '第 2、3 项校验失败', item: '第 2 项', missing: '未找到原文', duplicate: '原文有多处匹配', unchanged: '文件未改变', retry: '重新提交全部修改项' },
    { locale: 'en-US', summary: 'Edits 2, 3 failed validation', item: 'Edit 2', missing: 'Original text not found', duplicate: 'Original text matches multiple locations', unchanged: 'File unchanged', retry: 'resubmit all edits' },
  ])('locates failed items and explains the unchanged batch in $locale', (copy) => {
    setConfiguredLocale(copy.locale)
    const { container } = render(<ToolExecutionBlock message={{
      role: 'tool_call', name: 'edit', args: JSON.stringify({ path: 'state.md', edits }),
      status: 'error', result: errorResult(),
    }} />)
    expect(container.querySelector('[data-nova-tool-summary]')).toHaveTextContent(copy.summary)
    fireEvent.click(container.querySelector('[data-nova-tool-header]') as HTMLElement)
    const rows = container.querySelectorAll('[data-nova-edit-index]')
    expect(rows).toHaveLength(3)
    expect(rows[1]).toHaveTextContent(copy.item)
    expect(rows[1]).toHaveTextContent(copy.duplicate)
    expect(rows[1]).toHaveTextContent('same')
    expect(rows[2]).toHaveTextContent(copy.missing)
    expect(rows[0]).not.toHaveTextContent(copy.missing)
    expect(container.querySelector('[data-nova-tool-detail-output]')).toHaveTextContent(copy.duplicate)
    expect(container.querySelector('[data-nova-tool-detail-output]')).toHaveTextContent(copy.missing)
    expect(container).toHaveTextContent(copy.unchanged)
    expect(container).toHaveTextContent(copy.retry)
    expect(container).not.toHaveTextContent('workspace_change.tool_error.v1')
  })

  it('links overlap errors to the other edit and bounds the failed original-text preview', () => {
    const longText = '长原文'.repeat(400) + 'HIDDEN_PREVIEW_TAIL'
    const { container } = render(<ToolExecutionBlock message={{
      role: 'tool_call', name: 'edit',
      args: JSON.stringify({ path: 'state.md', edits: [edits[0], { old_string: longText, new_string: 'replacement' }] }),
      status: 'error', result: errorResult([{ edit_index: 1, code: 'overlap', details: { other_edit_index: 0 } }]),
    }} />)
    fireEvent.click(container.querySelector('[data-nova-tool-header]') as HTMLElement)
    const row = container.querySelector('[data-nova-edit-index="1"]')
    expect(row).toHaveTextContent('与第 1 项的修改范围重叠')
    expect(row).toHaveTextContent('原文预览已截短')
    expect(row).not.toHaveTextContent('HIDDEN_PREVIEW_TAIL')
  })

  it('does not describe successful or unstructured failures as a rejected batch', () => {
    const message = {
      role: 'tool_call' as const, name: 'edit', args: JSON.stringify({ path: 'state.md', edits }),
      status: 'success' as const, result: JSON.stringify({ schema: 'workspace_change.tool_result.v1', status: 'applied' }),
    }
    const { container, rerender } = render(<ToolExecutionBlock message={message} />)
    fireEvent.click(container.querySelector('[data-nova-tool-header]') as HTMLElement)
    expect(container).toHaveTextContent('已应用')
    expect(container).not.toHaveTextContent('文件未改变')
    rerender(<ToolExecutionBlock message={{ ...message, status: 'error', result: 'permission denied' }} />)
    expect(container).toHaveTextContent('permission denied')
    expect(container).not.toHaveTextContent('文件未改变')
  })

  it('preserves file-level errors without inventing item failures', () => {
    const { container } = render(<ToolExecutionBlock message={{
      role: 'tool_call', name: 'edit', args: JSON.stringify({ path: 'state.md', edits }), status: 'error',
      result: '[tool error]\n' + JSON.stringify({
        schema: 'workspace_change.tool_error.v1', code: 'invalid_edit', workspace_mutated: false,
        message: 'workspace file exceeds the mutation limit', details: { path: 'state.md', max_bytes: 16_777_216 },
      }),
    }} />)
    fireEvent.click(container.querySelector('[data-nova-tool-header]') as HTMLElement)
    expect(container).toHaveTextContent('workspace file exceeds the mutation limit')
    expect(container).not.toHaveTextContent('重新提交全部修改项')
  })
})
