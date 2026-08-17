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
            tool_name: 'bash', matcher_version: 1, command_key: '["go","test"]', command_pattern: 'go test ...',
            approved_args_hash: 'a'.repeat(64), approved_command: 'go test ./...', created_at: '2026-08-03T00:00:00Z',
          },
          {
            id: 'approval-two', scope: 'workspace', project_id: 'project-two', workspace: '/books/two',
            tool_name: 'bash', matcher_version: 1, command_key: '["git","push","origin"]', command_pattern: 'git push origin ...',
            approved_args_hash: 'b'.repeat(64), approved_command: 'git push origin main', created_at: '2026-08-03T00:00:00Z',
          },
        ]}
        onRevoke={onRevoke}
      />,
    )

    expect(screen.getByText('go test ...')).toBeInTheDocument()
    expect(screen.getByText('git push origin ...')).toBeInTheDocument()
    await user.click(screen.getAllByRole('button', { name: '撤销规则' })[0])
    expect(onRevoke).toHaveBeenCalledWith('approval-one')
  })
})
