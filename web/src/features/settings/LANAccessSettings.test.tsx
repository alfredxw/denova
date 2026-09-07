import { fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { setConfiguredLocale } from '@/i18n'
import { LANAccessSettings } from './LANAccessSettings'

vi.mock('@/features/remote-access/RemoteAccessConnections', () => ({ RemoteAccessConnections: () => null }))

beforeEach(() => setConfiguredLocale('en-US'))

describe('LANAccessSettings', () => {
  it('enables access and fills both credentials atomically, keeping the password available after autosave', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    const view = render(<LANAccessSettings draft={{}} inherited={{}} onChange={onChange} />)
    await user.click(screen.getByRole('combobox', { name: 'Allow LAN Access' }))
    await user.click(screen.getByRole('option', { name: 'On' }))
    expect(onChange).toHaveBeenCalledExactlyOnceWith({
      allow_lan_access: true,
      remote_access_username: expect.stringMatching(/^denova-.+/),
      remote_access_password: expect.stringMatching(/^[\w-]{24}$/),
    })
    const patch = onChange.mock.calls[0][0]
    // A successful save deliberately omits plaintext from the server snapshot.
    view.rerender(<LANAccessSettings draft={{ allow_lan_access: true, remote_access_username: patch.remote_access_username, remote_access_password_set: true }} inherited={{}} onChange={onChange} />)
    const password = screen.getByLabelText('Remote Access Password')
    expect(password).toHaveValue(patch.remote_access_password)
    expect(password).toHaveAttribute('type', 'text')
    await user.click(screen.getByRole('button', { name: 'Hide password' }))
    expect(password).toHaveAttribute('type', 'password')
    const copy = vi.spyOn(navigator.clipboard, 'writeText')
    await user.click(screen.getByRole('button', { name: 'Copy password' }))
    expect(copy).toHaveBeenCalledWith(patch.remote_access_password)
    expect(onChange).toHaveBeenCalledTimes(1)
    view.unmount()
    render(<LANAccessSettings draft={{ remote_access_password_set: true }} inherited={{}} onChange={onChange} />)
    expect(screen.getByLabelText('Remote Access Password')).toHaveValue('')
    expect(screen.getByRole('button', { name: 'Copy password' })).toBeDisabled()
  })

  it.each([
    { name: 'saved credentials', draft: { remote_access_username: 'reader', remote_access_password_set: true }, extra: {} },
    { name: 'entered credentials', draft: { remote_access_username: 'reader', remote_access_password: 'my-password' }, extra: {} },
    { name: 'existing username', draft: { remote_access_username: 'reader' }, extra: { remote_access_password: expect.any(String) } },
    { name: 'existing password', draft: { remote_access_password_set: true }, extra: { remote_access_username: expect.any(String) } },
  ])('preserves $name while filling only missing credentials', async ({ draft, extra }) => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(<LANAccessSettings draft={draft} inherited={{}} onChange={onChange} />)
    await user.click(screen.getByRole('combobox', { name: 'Allow LAN Access' }))
    await user.click(screen.getByRole('option', { name: 'On' }))
    expect(onChange).toHaveBeenCalledExactlyOnceWith({ allow_lan_access: true, ...extra })
  })

  it('does not generate credentials when disabling access', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(<LANAccessSettings draft={{}} inherited={{}} onChange={onChange} />)
    await user.click(screen.getByRole('combobox', { name: 'Allow LAN Access' }))
    await user.click(screen.getByRole('option', { name: 'Off' }))
    expect(onChange).toHaveBeenCalledExactlyOnceWith({ allow_lan_access: false })
  })

  it('allows replacing a generated password with an edited password', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(<LANAccessSettings draft={{ remote_access_password_set: true }} inherited={{}} onChange={onChange} />)
    await user.click(screen.getByRole('button', { name: 'Generate new password' }))
    expect(onChange).toHaveBeenCalledExactlyOnceWith({ remote_access_password: expect.stringMatching(/^[\w-]{24}$/) })
    fireEvent.change(screen.getByLabelText('Remote Access Password'), { target: { value: 'owner-edited-password' } })
    expect(onChange).toHaveBeenLastCalledWith({ remote_access_password: 'owner-edited-password' })
    expect(screen.getByLabelText('Remote Access Password')).toHaveValue('owner-edited-password')
  })
})
