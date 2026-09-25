import { useState } from 'react'
import { fireEvent, render, screen, within } from '@testing-library/react'
import { expect, it, vi } from 'vitest'
import { ImportResourcePicker } from './ImportResourcePicker'
import type { PackagePreview } from './api'
vi.mock('./ResourcePreview', () => ({ ResourcePreview: () => <p>Readable content</p> }))
const candidate: PackagePreview = {
  candidate_id: 'mixed', package: { id: 'mixed', name: 'Mixed' }, format: 'denova.resource-pack',
  resources: [
    { id: 'narrative', kind: 'preset.narrative', name: 'Quiet mystery', path: 'narrative.json', requires: ['style'], digest: '1' },
    { id: 'style', kind: 'style.reference', name: 'Prose style', path: 'style.md', digest: '2' },
    { id: 'ink', kind: 'preset.image', name: 'Ink', path: 'ink.json', description: 'Monochrome scenes', digest: '3' },
    { id: 'color', kind: 'preset.image', name: 'Color', path: 'color.json', digest: '4' },
  ],
}
function Picker({ pack = candidate, initial = [], onChange = () => {} }: {
  pack?: PackagePreview
  initial?: string[]
  onChange?: (ids: string[]) => void
}) {
  const [selected, setSelected] = useState(initial)
  return <ImportResourcePicker previewID="preview" candidate={pack} selected={selected} onChange={(ids) => { setSelected(ids); onChange(ids) }} />
}
it('keeps required resources selected and releases them with their parent', () => {
  render(<Picker />)
  fireEvent.click(screen.getByRole('checkbox', { name: 'Quiet mystery' }))
  expect(screen.getByText('已选 2 / 4 项')).toBeVisible()
  const dependency = screen.getByRole('checkbox', { name: 'Prose style' })
  expect(dependency).toBeChecked()
  expect(dependency).toBeDisabled()
  expect(screen.getByText('已自动包含：其他所选内容需要此资源')).toBeVisible()
  fireEvent.click(screen.getByRole('checkbox', { name: 'Quiet mystery' }))
  expect(dependency).not.toBeChecked()
  expect(dependency).toBeEnabled()
})
it('limits group selection to matches and preserves selections outside the search', () => {
  render(<Picker />)
  fireEvent.click(screen.getByRole('checkbox', { name: 'Quiet mystery' }))
  const search = screen.getByRole('textbox', { name: '搜索名称、描述或类型' })
  fireEvent.change(search, { target: { value: 'monochrome' } })
  const group = screen.getByRole('group', { name: '图像方案' })
  fireEvent.click(within(group).getByRole('checkbox', { name: '全选图像方案的搜索结果' }))
  expect(screen.getByText('已选 3 / 4 项')).toBeVisible()
  fireEvent.change(search, { target: { value: '' } })
  expect(screen.getByRole('checkbox', { name: 'Color' })).not.toBeChecked()
  expect(screen.getByRole('checkbox', { name: '全选图像方案' })).toBePartiallyChecked()
  fireEvent.click(screen.getByRole('button', { name: '预览 Ink' }))
  expect(within(screen.getByRole('dialog', { name: 'Ink' })).getByText('Readable content')).toBeVisible()
  fireEvent.click(screen.getByRole('button', { name: '关闭' }))
  fireEvent.click(screen.getByRole('button', { name: '清空选择' }))
  expect(screen.getByText('已选 0 / 4 项')).toBeVisible()
})

it.each([
  ['lore.item', '资料'],
  ['game.opening', '开场白'],
] as const)('selects %s as a whole group, with collapsed read-only previews and search-independent selection', (kind, label) => {
  const pack: PackagePreview = { ...candidate, resources: [
    ...candidate.resources,
    { id: 'harbor', kind, name: 'Harbor', path: 'harbor.json', digest: '5' },
    { id: 'island', kind, name: 'Island', path: 'island.json', digest: '6' },
  ] }
  const onChange = vi.fn()
  render(<Picker pack={pack} initial={['ink']} onChange={onChange} />)
  const group = screen.getByRole('group', { name: label })
  const checkbox = within(group).getByRole('checkbox', { name: `全选${label}` })
  const expand = within(group).getByRole('button', { name: label })
  expect(expand).toHaveAttribute('aria-expanded', 'false')
  expect(within(group).queryByText('Harbor')).not.toBeInTheDocument()
  fireEvent.click(expand)
  expect(within(group).getAllByRole('checkbox')).toHaveLength(1)
  expect(group.querySelectorAll('[aria-expanded]:not([aria-haspopup="dialog"])')).toHaveLength(1)
  fireEvent.click(within(group).getByRole('button', { name: '预览 Harbor' }))
  expect(within(screen.getByRole('dialog', { name: 'Harbor' })).getByText('Readable content')).toBeVisible()
  fireEvent.click(screen.getByRole('button', { name: '关闭' }))
  expect(expand).toHaveAttribute('aria-expanded', 'true')
  expect(onChange).not.toHaveBeenCalled()

  const search = screen.getByRole('textbox', { name: '搜索名称、描述或类型' })
  fireEvent.change(search, { target: { value: 'Harbor' } })
  expect(within(group).queryByText('Island')).not.toBeInTheDocument()
  fireEvent.click(checkbox)
  expect(onChange).toHaveBeenLastCalledWith(['ink', 'harbor', 'island'])
  expect(checkbox).toBeChecked()
  expect(within(group).getByText('2 / 2')).toBeVisible()
  fireEvent.click(checkbox)
  expect(onChange).toHaveBeenLastCalledWith(['ink'])
  fireEvent.change(search, { target: { value: 'no matching content' } })
  expect(screen.getByText('没有匹配的内容，已有选择保持不变。')).toBeVisible()
  expect(screen.getByText('已选 1 / 6 项')).toBeVisible()
})

it('preserves partial lore selections and required items without silently adding the rest of the group', () => {
  const pack: PackagePreview = { ...candidate, resources: [
    { id: 'game', kind: 'preset.game_planning', name: 'Harbor game', path: 'game.json', requires: ['harbor'], digest: '1' },
    { id: 'harbor', kind: 'lore.item', name: 'Harbor', path: 'harbor.json', digest: '2' },
    { id: 'island', kind: 'lore.item', name: 'Island', path: 'island.json', digest: '3' },
  ] }
  const onChange = vi.fn()
  render(<Picker pack={pack} initial={['game']} onChange={onChange} />)
  const checkbox = screen.getByRole('checkbox', { name: '全选资料' })
  expect(checkbox).toBePartiallyChecked()
  expect(screen.getByText('已选 2 / 3 项')).toBeVisible()
  expect(onChange).not.toHaveBeenCalled()
  fireEvent.click(checkbox)
  expect(onChange).toHaveBeenLastCalledWith(['game', 'harbor', 'island'])
  fireEvent.click(checkbox)
  expect(onChange).toHaveBeenLastCalledWith(['game'])
  expect(checkbox).toBePartiallyChecked()
  fireEvent.click(screen.getByRole('button', { name: '资料' }))
  expect(screen.getByText('已自动包含：其他所选内容需要此资源')).toBeVisible()
  fireEvent.click(screen.getByRole('button', { name: '清空选择' }))
  expect(checkbox).not.toBeChecked()
  expect(screen.getByText('已选 0 / 3 项')).toBeVisible()
})
