import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { SettingsDisclosureCard } from './SettingsDisclosureCard'

describe('SettingsDisclosureCard', () => {
  it('distinguishes connection and model levels while keeping each trigger independent', async () => {
    const user = userEvent.setup()
    render(
      <SettingsDisclosureCard
        level="connection"
        badge="Connection 1"
        title="Production"
        subtitle="2 models"
        defaultOpen
      >
        <SettingsDisclosureCard
          level="model"
          badge="Model 1"
          title="Writer"
          subtitle="gpt-example · Production"
          defaultOpen={false}
        >
          <div>Model settings body</div>
        </SettingsDisclosureCard>
      </SettingsDisclosureCard>,
    )

    const connectionTrigger = screen.getByRole('button', { name: /Connection 1/ })
    const modelTrigger = screen.getByRole('button', { name: /Model 1/ })

    expect(connectionTrigger).toHaveAttribute('aria-expanded', 'true')
    expect(modelTrigger).toHaveAttribute('aria-expanded', 'false')
    expect(connectionTrigger.closest('[data-settings-level="connection"]')).toBeInTheDocument()
    expect(modelTrigger.closest('[data-settings-level="model"]')).toBeInTheDocument()

    await user.click(modelTrigger)

    expect(modelTrigger).toHaveAttribute('aria-expanded', 'true')
    expect(screen.getByText('Model settings body')).toBeVisible()
  })
})
