import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { ConfirmDialog } from './ConfirmDialog'

describe('ConfirmDialog', () => {
  it('renders one detail and rich context, then keeps the dialog open when confirmation returns false', async () => {
    const onOpenChange = vi.fn()
    const onConfirm = vi.fn().mockResolvedValue(false)
    render(
      <ConfirmDialog
        open
        onOpenChange={onOpenChange}
        title="Delete book"
        description="This cannot be undone."
        confirmLabel="Delete"
        details={['/workspace/book']}
        detailContent={<p>Current book</p>}
        onConfirm={onConfirm}
      />,
    )

    expect(screen.getByText('/workspace/book')).toBeInTheDocument()
    expect(screen.getByText('Current book')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Delete' }))
    await waitFor(() => expect(onConfirm).toHaveBeenCalledOnce())
    expect(onOpenChange).not.toHaveBeenCalledWith(false)
    expect(screen.getByRole('alertdialog')).toBeInTheDocument()
  })

  it('cannot be dismissed with Escape while confirmation is pending and keeps errors visible', async () => {
    const confirmation = deferred<void>()
    const onOpenChange = vi.fn()
    render(
      <ConfirmDialog
        open
        onOpenChange={onOpenChange}
        title="Delete automation"
        description="This cannot be undone."
        confirmLabel="Delete"
        tone="danger"
        onConfirm={() => confirmation.promise}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Delete' }))
    expect(screen.getByRole('button', { name: 'Delete' })).toBeDisabled()
    fireEvent.keyDown(document, { key: 'Escape' })
    expect(onOpenChange).not.toHaveBeenCalledWith(false)
    expect(screen.getByRole('alertdialog')).toBeInTheDocument()

    await act(async () => {
      confirmation.reject(new Error('Delete failed'))
      try {
        await confirmation.promise
      } catch {
        // ConfirmDialog renders the rejection inline.
      }
    })
    expect(await screen.findByRole('alert')).toHaveTextContent('Delete failed')
    expect(screen.getByRole('alertdialog')).toBeInTheDocument()
  })
})

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}
