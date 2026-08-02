import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { DeleteConfirmDialog } from './DeleteConfirmDialog'

describe('DeleteConfirmDialog', () => {
  it('does not promise version recovery for project-scoped permanent deletion', () => {
    render(
      <DeleteConfirmDialog
        open
        path="src/main.ts"
        recovery="none"
        onOpenChange={vi.fn()}
        onConfirm={vi.fn()}
      />,
    )

    expect(screen.getByText('将永久删除 src/main.ts，且无法恢复。')).toBeInTheDocument()
    expect(screen.queryByText(/版本历史/)).not.toBeInTheDocument()
  })
})
