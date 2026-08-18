import type { ComponentProps } from 'react'
import { render, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it } from 'vitest'
import { setConfiguredLocale } from '@/i18n'
import { MessageItem as ProjectMessageItem } from './MessageItem'
import { ToolNavigationProvider, type ToolNavigationTarget } from './tool-navigation'

function MessageItem(props: ComponentProps<typeof ProjectMessageItem>) {
  return <ProjectMessageItem {...props} projectId={props.projectId || 'tool-detail-project'} />
}

async function expandToolCard(container: HTMLElement) {
  await userEvent.setup().click(container.querySelector('[data-nova-tool-header]') as HTMLElement)
}

describe('tool-specific details', () => {
  beforeEach(() => setConfiguredLocale('zh-CN'))

  it('read uses one compact path row and shows only the resource payload', async () => {
    const result = `${JSON.stringify({
      schema: 'resource.read.v1',
      status: 'success',
      source: { kind: 'local_text', path: 'chapters/ch01.md' },
      limits: { offset: 20, returned: 2, truncated: false },
    })}\n    20\t第一行\n    21\t  第二行\n    22\t\n`
    const { container } = render(
      <MessageItem message={{
        role: 'tool_call', name: 'read', content: 'read',
        args: JSON.stringify({ path: 'chapters/ch01.md', offset: 20, limit: 40 }),
        status: 'success', result,
      }} />,
    )

    await expandToolCard(container)

    const detail = container.querySelector('[data-nova-tool-detail="read"]') as HTMLElement
    const input = within(detail).getByText('chapters/ch01.md').parentElement as HTMLElement
    const output = detail.querySelector('[data-nova-tool-detail-output]') as HTMLElement
    expect(input).toHaveTextContent('offset=20')
    expect(input).toHaveTextContent('limit=40')
    expect([...output.querySelectorAll('[data-nova-read-line-number]')].map(element => element.textContent)).toEqual(['20', '21', '22'])
    expect([...output.querySelectorAll('[data-nova-read-line-content]')].map(element => element.textContent)).toEqual(['第一行', '  第二行', ''])
    expect(output.querySelector('[data-nova-read-line-number]')).toHaveClass('select-none', 'text-right')
    expect(output).not.toHaveTextContent('resource.read.v1')
    expect(detail).not.toHaveTextContent('输入')
    expect(detail).not.toHaveTextContent('输出')
  })

  it.each([
    {
      name: 'glob',
      args: { paths: ['chapters/**/*.md'], hidden: true, limit: 30 },
      expectedInput: 'chapters/**/*.md',
      expectedMeta: 'hidden=true',
      payload: 'chapters/ch01.md\nchapters/ch02.md',
    },
    {
      name: 'grep',
      args: { command: 'rg -n TODO chapters', description: '查找待办项' },
      expectedInput: 'rg -n TODO chapters',
      expectedMeta: '',
      payload: 'chapters/ch01.md:12:TODO',
    },
  ])('$name shows search arguments and hides the search envelope', async ({ name, args, expectedInput, expectedMeta, payload }) => {
    const result = `${JSON.stringify({
      schema: 'workspace.search.v1', status: 'success',
      source: { kind: name }, limits: { returned: 2, unit: 'paths', truncated: false },
    })}\n${payload}`
    const { container } = render(
      <MessageItem message={{ role: 'tool_call', name, content: name, args: JSON.stringify(args), status: 'success', result }} />,
    )

    await expandToolCard(container)

    const detail = container.querySelector(`[data-nova-tool-detail="${name}"]`) as HTMLElement
    expect(detail.querySelector('[data-nova-tool-detail-input]')).toHaveTextContent(expectedInput)
    if (expectedMeta) expect(detail).toHaveTextContent(expectedMeta)
    expect(detail.querySelector('[data-nova-tool-detail-output]')).toHaveTextContent(payload.replace(/\n/g, ' '))
    expect(detail).not.toHaveTextContent('workspace.search.v1')
  })

  it.each(['bash', 'pwsh'])('%s keeps the command compact and gives the output the remaining height', async (name) => {
    const result = `${JSON.stringify({
      schema: 'process.result.v1', status: 'success', shell: name,
      exit_code: 0, cwd: 'agent', pty: true, timeout_seconds: 30, output_truncated: false,
    })}\nok package/tools`
    const { container } = render(
      <MessageItem message={{
        role: 'tool_call', name, content: name,
        args: JSON.stringify({ command: 'go test ./tools', cwd: 'agent', pty: true, timeout_seconds: 30 }),
        status: 'success', result,
      }} />,
    )

    await expandToolCard(container)

    const detail = container.querySelector(`[data-nova-tool-detail="${name}"]`) as HTMLElement
    expect(detail.querySelector('[data-nova-tool-detail-input]')).toHaveTextContent('go test ./tools')
    expect(detail).toHaveTextContent('cwd=agent')
    expect(detail).toHaveTextContent('PTY')
    expect(detail).toHaveTextContent('timeout=30s')
    expect(detail.querySelector('[data-nova-tool-detail-output]')).toHaveTextContent('ok package/tools')
    expect(detail).not.toHaveTextContent('process.result.v1')
  })

  it('write prioritizes content while keeping a compact mutation receipt visible', async () => {
    const content = '章节正文。'.repeat(80)
    const result = JSON.stringify({
      schema: 'workspace_change.tool_result.v1', status: 'applied', path: 'chapters/ch01.md',
      review_status: 'pending', apply_state: 'applied',
      file_stats: { lines: 12, characters: 240, bytes: 480, non_whitespace_characters: 180 },
    })
    const { container } = render(
      <MessageItem message={{
        role: 'tool_call', name: 'write', content: 'write',
        args: JSON.stringify({ path: 'chapters/ch01.md', content }), status: 'success', result,
      }} />,
    )

    await expandToolCard(container)

    const detail = container.querySelector('[data-nova-tool-detail="write"]') as HTMLElement
    const input = detail.querySelector('[data-nova-tool-detail-input]') as HTMLElement
    const output = detail.querySelector('[data-nova-tool-detail-output]') as HTMLElement
    expect(input).toHaveClass('flex-1', 'overflow-y-auto')
    expect(input).toHaveTextContent('chapters/ch01.md')
    expect(input).toHaveTextContent('章节正文')
    expect(output).toHaveClass('max-h-20')
    expect(output).toHaveTextContent('已应用 · 12 行 · 240 字符')
    expect(detail).not.toHaveTextContent('workspace_change.tool_result.v1')
  })

  it('edit renders exact replacements as minus and plus blocks', async () => {
    const result = JSON.stringify({
      schema: 'workspace_change.tool_result.v1', status: 'applied', path: 'chapters/ch01.md',
      file_stats: { lines: 24, characters: 360 },
    })
    const { container } = render(
      <MessageItem message={{
        role: 'tool_call', name: 'edit', content: 'edit', status: 'success', result,
        args: JSON.stringify({
          path: 'chapters/ch01.md',
          edits: [{ old_string: '旧正文', new_string: '新正文', replace_all: true }],
        }),
      }} />,
    )

    await expandToolCard(container)

    const detail = container.querySelector('[data-nova-tool-detail="edit"]') as HTMLElement
    const input = detail.querySelector('[data-nova-tool-detail-input]') as HTMLElement
    expect(input).toHaveTextContent('−旧正文')
    expect(input).toHaveTextContent('+新正文')
    expect(input).toHaveTextContent('替换全部匹配')
    expect(detail.querySelector('[data-nova-tool-detail-output]')).toHaveTextContent('已应用 · 24 行 · 360 字符')
  })

  it.each([
    {
      name: 'read',
      args: { path: 'src/read.ts' },
      result: `${JSON.stringify({ schema: 'resource.read.v1' })}\n1\tcontent`,
    },
    {
      name: 'write',
      args: { path: 'src/write.ts', content: 'content' },
      result: JSON.stringify({ schema: 'workspace_change.tool_result.v1', file_stats: {} }),
    },
    {
      name: 'edit',
      args: { path: 'src/edit.ts', edits: [{ old_string: 'a', new_string: 'b' }] },
      result: JSON.stringify({ schema: 'workspace_change.tool_result.v1', file_stats: {} }),
    },
    {
      name: 'glob',
      args: { paths: ['src/**/*.ts'] },
      result: `${JSON.stringify({ schema: 'workspace.search.v1' })}\nsrc/glob.ts`,
    },
    {
      name: 'grep',
      args: { command: 'rg TODO src' },
      result: `${JSON.stringify({ schema: 'workspace.search.v1' })}\nsrc/grep.ts:12:TODO`,
    },
    {
      name: 'bash',
      args: { command: 'cat src/bash.ts' },
      result: `${JSON.stringify({ schema: 'process.result.v1' })}\ndone`,
    },
    {
      name: 'pwsh',
      args: { command: 'Get-Content .\\src\\pwsh.ts' },
      result: `${JSON.stringify({ schema: 'process.result.v1' })}\ndone`,
    },
  ])('$name opens inline workspace paths without collapsing the card', async ({ name, args, result }) => {
    const opened: ToolNavigationTarget[] = []
    const { container } = render(
      <ToolNavigationProvider value={{ workspace: '/workspace/book', open: (target) => opened.push(target) }}>
        <MessageItem message={{ role: 'tool_call', name, content: name, args: JSON.stringify(args), status: 'success', result }} />
      </ToolNavigationProvider>,
    )
    await expandToolCard(container)
    const link = container.querySelector('[data-nova-workspace-path]') as HTMLButtonElement
    expect(link).toBeInTheDocument()
    await userEvent.setup().click(link)
    expect(opened).toEqual([{ kind: 'workspace_file', path: expect.stringMatching(new RegExp(`${name}\\.ts$`)) }])
    expect(container.querySelector('[data-nova-tool-header]')).toHaveAttribute('aria-expanded', 'true')
  })

  it.each(['update_harness_state', 'initialize_story_state_schema', 'submit_director_plan_update'])(
    '%s keeps the default raw detail',
    async (name) => {
      const { container } = render(
        <MessageItem message={{ role: 'tool_call', name, content: name, args: '{"value":{"x":1}}', status: 'success', result: '{"status":"ok"}' }} />,
      )
      await expandToolCard(container)
      expect(container.querySelector(`[data-nova-tool-detail="${name}"]`)).not.toBeInTheDocument()
      expect(container.querySelector('[data-slot="collapsible-content"]')).toHaveTextContent('"value"')
    },
  )
})
