import { Check, FileText, Folder } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { FieldSet, FieldLegend } from '@/components/ui/field'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import type { ChapterStructure, ManuscriptNode } from '@/lib/api'

const structures: ChapterStructure[] = ['chapters', 'volumes', 'sections']

export function ChapterStructureSelector({ value, onChange, disabled, showExample = true }: {
  value: ChapterStructure
  onChange: (value: ChapterStructure) => void
  disabled?: boolean
  showExample?: boolean
}) {
  const { t } = useTranslation()
  const sample = value === 'chapters' ? ['chapter', 'chapter2'] : value === 'volumes' ? ['volume', 'chapter', 'chapter2'] : ['volume', 'chapter', 'section', 'section2']
  return (
    <FieldSet disabled={disabled}>
      <FieldLegend variant="label">{t('bookStructure.title')}</FieldLegend>
      <ToggleGroup type="single" variant="outline" size="sm" value={value} disabled={disabled}
        className="w-full flex-wrap items-stretch" aria-label={t('bookStructure.title')}
        onValueChange={(next) => { if (structures.includes(next as ChapterStructure)) onChange(next as ChapterStructure) }}>
        {structures.map((structure) => <ToggleGroupItem key={structure} value={structure} className="h-auto min-w-0 flex-1 basis-full whitespace-normal py-2 sm:basis-0">
          {value === structure && <Check className="size-3 shrink-0" />} {t(`bookStructure.${structure}`)}
        </ToggleGroupItem>)}
      </ToggleGroup>
      {showExample && <div className="flex flex-col gap-1 rounded-md border p-3 text-xs text-muted-foreground" aria-label={t('bookStructure.example')}>
        {sample.map((label, index) => {
          const depth = value === 'chapters' ? 0 : Math.min(index, value === 'sections' ? 2 : 1)
          const container = value !== 'chapters' && index < (value === 'sections' ? 2 : 1)
          const Icon = container ? Folder : FileText
          return <div key={label} className="flex items-center gap-2" style={{ paddingInlineStart: depth * 16 }}><Icon className="size-3.5 shrink-0" />{t(`bookStructure.example.${label}`)}</div>
        })}
      </div>}
    </FieldSet>
  )
}

export function ManuscriptPreviewTree({ nodes }: { nodes: ManuscriptNode[] }) {
  return <ul className="flex min-w-0 flex-col gap-1 text-xs">
    {nodes.map((node) => <li key={node.path} className="min-w-0">
      <div className="flex min-w-0 items-center gap-1.5" title={node.path}>
        {node.directory ? <Folder className="size-3 shrink-0" /> : <FileText className="size-3 shrink-0" />}
        <span className="truncate">{node.title}</span>
      </div>
      {node.children?.length ? <div className="mt-1 min-w-0 pl-4"><ManuscriptPreviewTree nodes={node.children} /></div> : null}
    </li>)}
  </ul>
}
