import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Field, SwitchWithInheritance } from './agent-form-controls'

describe('Agent inheritance controls', () => {
  it('separates a configured value from the action that restores inheritance', async () => {
    const reset = vi.fn()
    render(<Field label="执行引擎" inherited={false} onReset={reset}><span>Native</span></Field>)

    expect(screen.queryByRole('button', { name: '覆盖' })).not.toBeInTheDocument()
    expect(reset).not.toHaveBeenCalled()
    await userEvent.click(screen.getByRole('button', { name: '恢复执行引擎的继承设置' }))
    expect(reset).toHaveBeenCalledOnce()
  })

  it('names the field restored by a tool permission action', async () => {
    const reset = vi.fn()
    const change = vi.fn()
    render(<SwitchWithInheritance checked onChange={change} ariaLabel="读取文件" inherited={false} onReset={reset} />)
    await userEvent.click(screen.getByRole('button', { name: '恢复读取文件的继承设置' }))
    expect(reset).toHaveBeenCalledOnce()
    expect(change).not.toHaveBeenCalled()
  })
})
