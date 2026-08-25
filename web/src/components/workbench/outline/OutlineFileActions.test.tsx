import { fireEvent, render, screen } from '@testing-library/react'
import { OutlineFileActions } from './OutlineFileActions'

describe('OutlineFileActions', () => {
  it('shows the same platform shortcuts as project files and handles delete from the row', () => {
    render(
      <OutlineFileActions
        path="chapters/ch00001-opening.md"
        fileOperations={{
          projectId: 'project-1',
          workspace: '/workspace/book',
          getExistingPaths: () => [],
        }}
        onDeleteItem={vi.fn(async () => {})}
      >
        <button type="button">Opening</button>
      </OutlineFileActions>,
    )

    const row = screen.getByRole('button', { name: 'Opening' })
    fireEvent.contextMenu(row, { clientX: 20, clientY: 20 })

    expect(screen.getByText('Ctrl+Alt+C')).toBeInTheDocument()
    expect(screen.getByText('Ctrl+Alt+Shift+C')).toBeInTheDocument()
    expect(screen.getByText('Delete')).toBeInTheDocument()

    fireEvent.keyDown(row, { key: 'Delete' })
    expect(screen.getByRole('alertdialog')).toBeInTheDocument()
  })
})
