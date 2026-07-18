import { useState } from 'react'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { CompactResourcePicker } from './CompactResourcePicker'

const resources = [
  { id: 'first', name: 'First resource' },
  { id: 'second', name: 'Second resource' },
]

describe('CompactResourcePicker', () => {
  it('lets the inline trigger shrink or wrap inside narrow toolbars', () => {
    const { container } = render(<Picker items={resources} selectedId="first" onSelect={() => undefined} />)

    expect(container.querySelector('[data-layout="inline"]')).toHaveClass('w-full', 'flex-wrap', 'min-w-0')
    expect(screen.getByRole('button', { name: 'Choose resource' })).toHaveClass('w-full', 'min-w-0')
  })

  it('selects a resource and closes the popover', async () => {
    const user = userEvent.setup()
    const onSelect = vi.fn()
    render(<Picker items={resources} selectedId="first" onSelect={onSelect} />)

    await user.click(screen.getByRole('button', { name: 'Choose resource' }))
    await user.click(screen.getByRole('button', { name: 'Second resource' }))

    expect(onSelect).toHaveBeenCalledWith('second')
    expect(screen.queryByRole('button', { name: 'Second resource' })).not.toBeInTheDocument()
  })

  it('does not open while disabled and explains an empty catalog', async () => {
    const user = userEvent.setup()
    const view = render(<Picker items={resources} selectedId="first" disabled onSelect={() => undefined} />)

    await user.click(screen.getByRole('button', { name: 'Choose resource' }))
    expect(screen.queryByRole('button', { name: 'Second resource' })).not.toBeInTheDocument()

    view.rerender(<Picker items={[]} onSelect={() => undefined} />)
    await user.click(screen.getByRole('button', { name: 'Choose resource' }))
    expect(screen.getByText('No resources')).toBeInTheDocument()
  })

  it('supports deleting the current resource from a domain footer', async () => {
    const user = userEvent.setup()
    render(<DeletingPicker />)

    expect(screen.getByRole('button', { name: 'Choose resource' })).toHaveTextContent('First resource')
    await user.click(screen.getByRole('button', { name: 'Choose resource' }))
    await user.click(screen.getByRole('button', { name: 'Delete current resource' }))

    expect(screen.getByRole('button', { name: 'Choose resource' })).toHaveTextContent('Choose one')
    expect(screen.queryByRole('button', { name: 'Delete current resource' })).not.toBeInTheDocument()
  })
})

function Picker({
  items,
  selectedId,
  disabled,
  onSelect,
}: {
  items: typeof resources
  selectedId?: string
  disabled?: boolean
  onSelect: (id: string) => void
}) {
  return (
    <CompactResourcePicker
      items={items}
      selectedId={selectedId}
      getId={(item) => item.id}
      getLabel={(item) => item.name}
      label="Resource"
      ariaLabel="Choose resource"
      placeholder="Choose one"
      emptyLabel="No resources"
      disabled={disabled}
      onSelect={onSelect}
    />
  )
}

function DeletingPicker() {
  const [items, setItems] = useState(resources)
  const [selectedId, setSelectedId] = useState<string | undefined>('first')
  return (
    <CompactResourcePicker
      items={items}
      selectedId={selectedId}
      getId={(item) => item.id}
      getLabel={(item) => item.name}
      label="Resource"
      ariaLabel="Choose resource"
      placeholder="Choose one"
      emptyLabel="No resources"
      onSelect={setSelectedId}
      renderFooter={(close) => (
        <button
          type="button"
          onClick={() => {
            setItems((current) => current.filter((item) => item.id !== selectedId))
            setSelectedId(undefined)
            close()
          }}
        >
          Delete current resource
        </button>
      )}
    />
  )
}
