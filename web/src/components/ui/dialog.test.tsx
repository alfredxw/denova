import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { AlertDialog, AlertDialogContent, AlertDialogDescription, AlertDialogTitle } from './alert-dialog'
import { Dialog, DialogContent, DialogTitle } from './dialog'

describe('dialog primitives', () => {
  it('uses a wider adaptive default for regular dialogs', () => {
    render(
      <Dialog open>
        <DialogContent showCloseButton={false}>
          <DialogTitle>Dialog title</DialogTitle>
        </DialogContent>
      </Dialog>,
    )

    const dialog = screen.getByRole('dialog')
    expect(dialog.className).toContain('max-w-[min(calc(100vw-2rem),42rem)]')
    expect(dialog.className).not.toContain('max-w-sm')
  })

  it('allows explicit dialog widths to override the adaptive default', () => {
    render(
      <Dialog open>
        <DialogContent showCloseButton={false} className="max-w-5xl">
          <DialogTitle>Wide dialog</DialogTitle>
        </DialogContent>
      </Dialog>,
    )

    const dialog = screen.getByRole('dialog')
    expect(dialog).toHaveClass('max-w-5xl')
    expect(dialog.className).not.toContain('max-w-[min(calc(100vw-2rem),42rem)]')
  })

  it('uses a wider adaptive default for alert dialogs', () => {
    render(
      <AlertDialog open>
        <AlertDialogContent>
          <AlertDialogTitle>Alert title</AlertDialogTitle>
          <AlertDialogDescription>Confirm the action.</AlertDialogDescription>
        </AlertDialogContent>
      </AlertDialog>,
    )

    const dialog = screen.getByRole('alertdialog')
    expect(dialog.className).toContain('max-w-[min(calc(100vw-2rem),36rem)]')
    expect(dialog.className).not.toContain('max-w-xs')
  })

  it('keeps small alert dialogs compact but not cramped', () => {
    render(
      <AlertDialog open>
        <AlertDialogContent size="sm">
          <AlertDialogTitle>Small alert</AlertDialogTitle>
          <AlertDialogDescription>Confirm the action.</AlertDialogDescription>
        </AlertDialogContent>
      </AlertDialog>,
    )

    const dialog = screen.getByRole('alertdialog')
    expect(dialog.className).toContain('max-w-[min(calc(100vw-2rem),30rem)]')
    expect(dialog.className).not.toContain('max-w-xs')
  })
})
