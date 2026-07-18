import { fireEvent, render, screen } from '@testing-library/react'
import { Settings } from 'lucide-react'
import { describe, expect, it, vi } from 'vitest'
import { FeaturePageShell } from './feature-page-shell'

describe('FeaturePageShell', () => {
  it('provides one semantic title and consistent actions, errors, and close behavior', () => {
    const onClose = vi.fn()
    render(
      <FeaturePageShell
        icon={Settings}
        title="Settings"
        subtitle="Workspace preferences"
        actions={<button type="button">Save</button>}
        error="Could not save"
        errorTitle="Save failed"
        closeLabel="Close settings"
        onClose={onClose}
      >
        <main>Settings body</main>
      </FeaturePageShell>,
    )

    expect(screen.getByRole('heading', { level: 2, name: 'Settings' })).toBeInTheDocument()
    expect(screen.getByText('Workspace preferences')).toBeInTheDocument()
    expect(screen.getByText('Could not save')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Save' })).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Close settings' }))
    expect(onClose).toHaveBeenCalledOnce()
  })

  it('keeps leading navigation and actions outside the truncating long-title region', () => {
    render(
      <FeaturePageShell
        icon={Settings}
        title="A very long dynamically generated resource title that must not hide controls"
        leadingContent={<button type="button">Open navigation</button>}
        actions={<button type="button">Save</button>}
      >
        <main>Body</main>
      </FeaturePageShell>,
    )

    const heading = screen.getByRole('heading', { level: 2 })
    const leading = screen.getByRole('button', { name: 'Open navigation' })
    const action = screen.getByRole('button', { name: 'Save' })
    expect(heading).toHaveClass('min-w-0', 'truncate')
    expect(leading.compareDocumentPosition(heading) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    expect(heading.compareDocumentPosition(action) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  })
})
