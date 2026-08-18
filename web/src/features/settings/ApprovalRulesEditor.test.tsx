import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { ApprovalRulesEditor } from './ApprovalRulesEditor'

describe('ApprovalRulesEditor', () => {
  it('shows the saved scope and revokes the selected stable rule ID', async () => {
    const user = userEvent.setup()
    const onRevoke = vi.fn()
    render(
      <ApprovalRulesEditor
        rules={[
          {
            id: 'approval-one', scope: 'workspace', project_id: 'project-one', workspace: '/books/one',
            tool_name: 'bash', matcher: 'shell_command', matcher_version: 1, match_key: '["go","test"]', display_pattern: 'go test ...',
            approved_args_hash: 'a'.repeat(64), approved_input: 'go test ./...', created_at: '2026-08-03T00:00:00Z',
          },
          {
            id: 'approval-two', scope: 'workspace', project_id: 'project-two', workspace: '/books/two',
            tool_name: 'bash', matcher: 'shell_command', matcher_version: 1, match_key: '["git","push","origin"]', display_pattern: 'git push origin ...',
            approved_args_hash: 'b'.repeat(64), approved_input: 'git push origin main', created_at: '2026-08-03T00:00:00Z',
          },
          {
            id: 'approval-three', scope: 'workspace', project_id: 'project-one', workspace: '/books/one',
            tool_name: 'filesystem_read', matcher: 'filesystem_read_root', matcher_version: 1,
            match_key: '[{"path":"D:/Shared","recursive":true}]', display_pattern: 'D:/Shared/**',
            approved_args_hash: 'c'.repeat(64), approved_input: 'D:/Shared/**', created_at: '2026-08-03T00:00:00Z',
          },
        ]}
        onRevoke={onRevoke}
      />,
    )

    expect(screen.getByText('go test ...')).toBeInTheDocument()
    expect(screen.getByText('git push origin ...')).toBeInTheDocument()
    expect(screen.getByText('D:/Shared/**')).toBeInTheDocument()
    await user.click(screen.getAllByRole('button', { name: '撤销规则' })[0])
    expect(onRevoke).toHaveBeenCalledWith('approval-one')
  })
})
