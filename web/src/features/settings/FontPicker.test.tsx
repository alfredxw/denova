import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { FontPicker } from './FontPicker'

afterEach(() => {
  Reflect.deleteProperty(window, 'queryLocalFonts')
})

describe('FontPicker', () => {
  it('uses the search field as a custom font option without a separate preview or apply action', async () => {
    const user = userEvent.setup()
    const onValueChange = vi.fn()
    render(<FontPicker value="apple-system" onValueChange={onValueChange} />)

    await user.click(screen.getByRole('combobox', { name: '选择字体，当前：Apple / 苹方' }))
    await user.type(screen.getByPlaceholderText('搜索或输入字体名称'), 'Fira Sans')

    const customOption = within(screen.getByRole('group', { name: '自定义字体' }))
      .getByRole('option', { name: 'Fira Sans' })
    expect(customOption.firstElementChild?.getAttribute('style')).toContain('Fira Sans')
    expect(screen.queryByText('字体预览')).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '使用' })).not.toBeInTheDocument()

    await user.click(customOption)
    expect(onValueChange).toHaveBeenCalledWith('custom:Fira Sans')
  })

  it('selects a built-in font preset', async () => {
    const user = userEvent.setup()
    const onValueChange = vi.fn()
    render(<FontPicker value="apple-system" onValueChange={onValueChange} />)

    const trigger = screen.getByRole('combobox', { name: '选择字体，当前：Apple / 苹方' })
    expect(within(trigger).getByText('Apple / 苹方').getAttribute('style')).toContain('SF Pro Text')

    await user.click(trigger)
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
    const localFonts = await screen.findByRole('group', { name: '本机字体' })
    await user.click(within(localFonts).getByText('霞鹜文楷'))

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
