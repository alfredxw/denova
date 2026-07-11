import { fireEvent, render, screen } from '@testing-library/react'
import { useState } from 'react'
import { describe, expect, it, vi } from 'vitest'
import type {
  EventPackageModule,
  StoryDirectorActorStateSystem,
  StoryMemoryStructure,
} from '../../types'
import {
  ActorStateVisualEditor,
  EventPackageVisualEditor,
  MemoryStructureVisualEditor,
} from './visual-editors'

describe('preset visual editor selection stability', () => {
  it('keeps the edited event card selected when its id changes', async () => {
    function Harness() {
      const [value, setValue] = useState<EventPackageModule>({
        version: 1,
        id: 'events',
        name: 'Events',
        description: '',
        custom: true,
        events: [
          { id: 'event_one', type_name: 'First Event', enabled: true },
          { id: 'event_two', type_name: 'Second Event', enabled: true },
        ],
      })
      return <EventPackageVisualEditor value={value} onChange={setValue} onValidityChange={vi.fn()} />
    }

    render(<Harness />)
    const idInput = await screen.findByRole('textbox', { name: 'ID' })

    fireEvent.change(idInput, { target: { value: 'event_renamed' } })

    expect(screen.getByRole('textbox', { name: 'ID' })).toHaveValue('event_renamed')
    expect(screen.getByRole('textbox', { name: '事件类型名' })).toHaveValue('First Event')
    expect(screen.getByTestId('event-package-cards-trigger-event_renamed')).toHaveAttribute('data-state', 'active')
  })

  it('renames an actor template atomically with its initial actor references', async () => {
    let latest: StoryDirectorActorStateSystem | undefined
    function Harness() {
      const [value, setValue] = useState<StoryDirectorActorStateSystem>({
        templates: [{ id: 'hero', name: 'Hero', fields: [] }],
        initial_actors: [{ id: 'player', name: 'Player', template_id: 'hero', role: 'lead', state: {} }],
      })
      latest = value
      return <ActorStateVisualEditor value={value} onChange={setValue} onValidityChange={vi.fn()} />
    }

    render(<Harness />)
    const templateIdInput = (await screen.findAllByRole('textbox', { name: 'ID' }))[0]

    fireEvent.change(templateIdInput, { target: { value: 'protagonist' } })

    expect(latest?.templates?.[0].id).toBe('protagonist')
    expect(latest?.initial_actors?.[0].template_id).toBe('protagonist')
    expect(screen.getByTestId('actor-state-templates-trigger-protagonist')).toHaveAttribute('data-state', 'active')
    expect(screen.getByDisplayValue('player')).toBeInTheDocument()
  })

  it('keeps a memory structure and field open while cascading a keyed field id', async () => {
    let latest: StoryMemoryStructure[] = []
    function Harness() {
      const [value, setValue] = useState<StoryMemoryStructure[]>([{
        id: 'memory_one',
        name: 'Memory One',
        mode: 'keyed',
        key_field_id: 'key_one',
        fields: [{ id: 'key_one', name: 'Key One', order: 10 }],
        order: 10,
      }])
      latest = value
      return <MemoryStructureVisualEditor value={value} onChange={setValue} onValidityChange={vi.fn()} resetKey="memory" />
    }

    render(<Harness />)
    const structureIdInput = await screen.findByDisplayValue('memory_one')
    const fieldIdInput = await screen.findByDisplayValue('key_one')

    fireEvent.change(structureIdInput, { target: { value: 'memory_renamed' } })
    fireEvent.change(fieldIdInput, { target: { value: 'key_renamed' } })

    expect(latest[0].id).toBe('memory_renamed')
    expect(latest[0].fields[0].id).toBe('key_renamed')
    expect(latest[0].key_field_id).toBe('key_renamed')
    expect(screen.getByTestId('memory-structures-trigger-memory_renamed')).toHaveAttribute('data-state', 'active')
    expect(screen.getByDisplayValue('key_renamed')).toBeInTheDocument()
  })
})
