import { fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ImagePreviewDialog } from './ImagePreviewDialog'

const VIEWPORT_WIDTH = 1_000
const VIEWPORT_HEIGHT = 800

describe('ImagePreviewDialog interactions', () => {
  beforeEach(() => {
    vi.spyOn(HTMLElement.prototype, 'offsetWidth', 'get').mockReturnValue(VIEWPORT_WIDTH)
    vi.spyOn(HTMLElement.prototype, 'offsetHeight', 'get').mockReturnValue(VIEWPORT_HEIGHT)
    vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockReturnValue({
      width: VIEWPORT_WIDTH,
      height: VIEWPORT_HEIGHT,
      top: 0,
      left: 0,
      right: VIEWPORT_WIDTH,
      bottom: VIEWPORT_HEIGHT,
      x: 0,
      y: 0,
      toJSON: () => ({}),
    } as DOMRect)
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('zooms around the pointer with an unmodified wheel gesture', () => {
    const viewport = renderOpenViewer()

    expect(screen.getByText('100%')).toBeInTheDocument()
    fireEvent.wheel(viewport, { deltaY: -100, clientX: 500, clientY: 400 })
    expect(screen.getByText('120%')).toBeInTheDocument()

    fireEvent.wheel(viewport, { deltaY: 100, clientX: 500, clientY: 400 })
    fireEvent.wheel(viewport, { deltaY: -100, clientX: 500, clientY: 400, ctrlKey: true })
    expect(screen.getByText('120%')).toBeInTheDocument()
  })

  it('toggles between fit and 2x zoom on double click', async () => {
    const viewport = renderOpenViewer()

    fireEvent.doubleClick(viewport, { clientX: 500, clientY: 400 })
    expect(screen.getByText('200%')).toBeInTheDocument()

    await new Promise((resolve) => setTimeout(resolve, 0))
    fireEvent.doubleClick(viewport, { clientX: 500, clientY: 400 })
    expect(screen.getByText('100%')).toBeInTheDocument()
  })

  it('pans with primary-button drag without taking over secondary-button drag', () => {
    const viewport = renderOpenViewer()
    const content = viewport.querySelector('.react-transform-component') as HTMLElement

    fireEvent.doubleClick(viewport, { clientX: 500, clientY: 400 })
    const centeredTransform = content.style.transform

    fireEvent.mouseDown(viewport, { button: 2, buttons: 2, clientX: 500, clientY: 400 })
    fireEvent.mouseMove(window, { buttons: 2, clientX: 600, clientY: 400 })
    fireEvent.mouseUp(window, { button: 2, buttons: 0, clientX: 600, clientY: 400 })
    expect(content.style.transform).toBe(centeredTransform)

    fireEvent.mouseDown(viewport, { button: 0, buttons: 1, clientX: 500, clientY: 400 })
    fireEvent.mouseMove(window, { buttons: 1, clientX: 600, clientY: 400 })
    expect(content.style.transform).not.toBe(centeredTransform)
    fireEvent.mouseUp(window, { button: 0, buttons: 0, clientX: 600, clientY: 400 })
  })

  it('supports keyboard zoom, reset, and bounded arrow-key panning', () => {
    const viewport = renderOpenViewer()
    const dialog = screen.getByRole('dialog', { name: 'Image preview' })
    const content = viewport.querySelector('.react-transform-component') as HTMLElement

    fireEvent.keyDown(dialog, { key: '+', shiftKey: true })
    expect(screen.getByText('125%')).toBeInTheDocument()

    const centeredTransform = content.style.transform
    fireEvent.keyDown(dialog, { key: 'ArrowRight' })
    expect(content.style.transform).not.toBe(centeredTransform)

    fireEvent.keyDown(dialog, { key: '0' })
    expect(screen.getByText('100%')).toBeInTheDocument()

    fireEvent.keyDown(dialog, { key: '-' })
    expect(screen.getByText('80%')).toBeInTheDocument()
  })
})

function renderOpenViewer(): HTMLElement {
  render(
    <ImagePreviewDialog src="/image.png" title="Image preview" alt="Preview image">
      <button type="button">Open image</button>
    </ImagePreviewDialog>,
  )
  fireEvent.click(screen.getByRole('button', { name: 'Open image' }))
  return screen.getByTestId('image-preview-viewport')
}
