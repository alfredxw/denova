import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { FontPicker } from './FontPicker'

afterEach(() => {
  Reflect.deleteProperty(window, 'queryLocalFonts')
})

describe('FontPicker', () => {
  it('applies a manually entered family with a live preview', async () => {
    const user = userEvent.setup()
    const onValueChange = vi.fn()
    render(<FontPicker value="apple-system" onValueChange={onValueChange} />)

    await user.click(screen.getByRole('combobox', { name: '选择字体，当前：Apple / 苹方' }))
    await user.type(screen.getByLabelText('字体家族名称'), 'Microsoft YaHei')

    expect(screen.getByText('创作，从此刻开始。').getAttribute('style')).toContain('Microsoft YaHei')
    await user.click(screen.getByRole('button', { name: '使用' }))
    expect(onValueChange).toHaveBeenCalledWith('custom:Microsoft YaHei')
  })

  it('selects a built-in font preset', async () => {
    const user = userEvent.setup()
    const onValueChange = vi.fn()
    render(<FontPicker value="apple-system" onValueChange={onValueChange} />)

    await user.click(screen.getByRole('combobox', { name: '选择字体，当前：Apple / 苹方' }))
    await user.click(screen.getByRole('option', { name: '系统无衬线（推荐）' }))

    expect(onValueChange).toHaveBeenCalledWith('system-sans')
  })

  it('deduplicates browser font faces and selects their family', async () => {
    const user = userEvent.setup()
    const onValueChange = vi.fn()
    const queryLocalFonts = vi.fn().mockResolvedValue([
      { family: '霞鹜文楷', fullName: '霞鹜文楷 Regular', postscriptName: 'LXGWWenKai-Regular', style: 'Regular' },
      { family: '霞鹜文楷', fullName: '霞鹜文楷 Bold', postscriptName: 'LXGWWenKai-Bold', style: 'Bold' },
    ])
    Object.defineProperty(window, 'queryLocalFonts', { configurable: true, value: queryLocalFonts })
    render(<FontPicker value="apple-system" onValueChange={onValueChange} />)

    await user.click(screen.getByRole('combobox', { name: '选择字体，当前：Apple / 苹方' }))
    await user.click(screen.getByRole('button', { name: '读取本机字体' }))
    expect(await screen.findByText('已读取 1 个本机字体家族')).toBeInTheDocument()
    await user.click(within(screen.getByRole('group', { name: '本机字体' })).getByText('霞鹜文楷'))

    expect(queryLocalFonts).toHaveBeenCalledTimes(1)
    expect(onValueChange).toHaveBeenCalledWith('custom:霞鹜文楷')
  })

  it('keeps user-layer inheritance as an explicit choice', async () => {
    const user = userEvent.setup()
    const onValueChange = vi.fn()
    render(<FontPicker value="source-han-serif" inherited="apple-system" allowInherit onValueChange={onValueChange} />)

    await user.click(screen.getByRole('combobox', { name: '选择字体，当前：思源宋体阅读' }))
    await user.click(screen.getByText('继承（Apple / 苹方）'))
    expect(onValueChange).toHaveBeenCalledWith('')
  })
})
