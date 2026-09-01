import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ScrollText, Wrench } from 'lucide-react'

import { AgentConfigurationDisclosure } from './agent-configuration-disclosure'

describe('AgentConfigurationDisclosure', () => {
  it('keeps modules independently collapsible and toggles from the full header', async () => {
    const user = userEvent.setup()
    render(
      <>
        <AgentConfigurationDisclosure
          id="behavior"
          icon={ScrollText}
          title="Behavior"
          summary="Prompts and rules"
          defaultOpen
        >
          <div>Behavior settings</div>
        </AgentConfigurationDisclosure>
        <AgentConfigurationDisclosure
          id="capabilities"
          icon={Wrench}
          title="Capabilities"
          summary="4 enabled tools"
        >
          <div>Capability settings</div>
        </AgentConfigurationDisclosure>
      </>,
    )

    const behaviorTrigger = screen.getByRole('button', { name: 'Behavior' })
    const capabilityTrigger = screen.getByRole('button', { name: 'Capabilities' })

    expect(behaviorTrigger).toHaveAttribute('aria-expanded', 'true')
    expect(capabilityTrigger).toHaveAttribute('aria-expanded', 'false')
    expect(behaviorTrigger.closest('[data-agent-configuration-section="behavior"]')).toBeInTheDocument()
    expect(capabilityTrigger.closest('[data-agent-configuration-section="capabilities"]')).toBeInTheDocument()

    await user.click(capabilityTrigger)

    expect(behaviorTrigger).toHaveAttribute('aria-expanded', 'true')
    expect(capabilityTrigger).toHaveAttribute('aria-expanded', 'true')
    expect(screen.getByText('Capability settings')).toBeVisible()
  })
})
