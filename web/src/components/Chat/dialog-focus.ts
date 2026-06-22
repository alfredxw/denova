export function focusDialogContentOnOpen(event: Event) {
  event.preventDefault()
  const dialog = event.currentTarget as HTMLElement | null
  window.requestAnimationFrame(() => {
    dialog?.focus({ preventScroll: true })
  })
}
