import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { Input } from '@/components/ui/input'
import { FormField } from './form-field'

describe('FormField', () => {
  it('names compound groups and associates ordinary labels with their controls', () => {
    render(
      <>
        <FormField label="Delivery" semanticGroup description="Choose one option" error="Selection required">
          <button type="button">Daily</button>
        </FormField>
        <FormField label="Display name" htmlFor="display-name">
          <Input id="display-name" />
        </FormField>
      </>,
    )

    const group = screen.getByRole('group', { name: 'Delivery' })
    expect(group).toHaveAccessibleDescription('Choose one option Selection required')
    expect(screen.getByRole('textbox', { name: 'Display name' })).toBeInTheDocument()
  })
})
