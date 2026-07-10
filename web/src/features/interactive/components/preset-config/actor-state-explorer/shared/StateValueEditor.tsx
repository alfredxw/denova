import { AnimatePresence, motion } from 'motion/react'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Braces, Eye } from 'lucide-react'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { novaEase } from '@/features/motion/motion-tokens'
import { cn } from '@/lib/utils'
import { formatPresetJSON, parseNumberInput } from '../../utils'
import { KeyValEditor, type KeyValEntry } from './KeyValEditor'

interface StateValueEditorProps {
  type: string
  value: unknown
  onChange: (value: unknown) => void
  options?: string[]
  min?: number
  max?: number
  label?: string
  compact?: boolean
}

export function StateValueEditor({
  type,
  value,
  onChange,
  options = [],
  min,
  max,
  label,
  compact = false,
}: StateValueEditorProps) {
  const { t } = useTranslation()
  const [viewMode, setViewMode] = useState<'structured' | 'json'>('structured')
  const [jsonText, setJsonText] = useState(() => formatPresetJSON(value ?? getDefaultForType(type)))
  const [jsonError, setJsonError] = useState('')

  useEffect(() => {
    setJsonText(formatPresetJSON(value ?? getDefaultForType(type)))
    setJsonError('')
  }, [type, value])

  const needsJsonFallback = type === 'object' || type === 'list'

  const handleJsonChange = (text: string) => {
    setJsonText(text)
    try {
      const parsed = JSON.parse(text)
      if (type === 'list' && !Array.isArray(parsed)) {
        throw new Error(t('settingPanel.actorState.explorer.arrayRequired'))
      }
      if (type === 'object' && (typeof parsed !== 'object' || Array.isArray(parsed))) {
        throw new Error(t('settingPanel.actorState.explorer.objectRequired'))
      }
      setJsonError('')
      onChange(parsed)
    } catch (err) {
      setJsonError(err instanceof Error ? err.message : t('settingPanel.actorState.explorer.invalidJSON'))
    }
  }

  if (needsJsonFallback && viewMode === 'json') {
    return (
      <div className="space-y-1.5">
        {label ? <FieldLabel>{label}</FieldLabel> : null}
        <div className="flex items-center justify-end gap-1">
          <button
            type="button"
            className={cn(
              'flex h-7 items-center gap-1 rounded-full px-2 text-[10px] transition-colors',
              'text-[var(--nova-text-faint)] hover:text-[var(--nova-text)]',
            )}
            onClick={() => setViewMode('structured')}
          >
            <Eye className="h-3 w-3" />
            {t('settingPanel.actorState.explorer.structured')}
          </button>
          <button
            type="button"
            className={cn(
              'flex h-7 items-center gap-1 rounded-full px-2 text-[10px] transition-colors',
              'bg-[var(--nova-text)] text-[var(--nova-surface)]',
            )}
            onClick={() => setViewMode('json')}
          >
            <Braces className="h-3 w-3" />
            JSON
          </button>
        </div>
        <Textarea
          className="nova-field min-h-24 resize-y font-mono text-xs leading-5 shadow-none focus-visible:ring-0"
          value={jsonText}
          onChange={(e) => handleJsonChange(e.target.value)}
        />
        {jsonError ? (
          <div className="rounded-[var(--nova-radius)] border border-[var(--nova-danger-border)] bg-[var(--nova-danger-bg)] px-2 py-1 text-[11px] text-[var(--nova-danger)]">
            {jsonError}
          </div>
        ) : null}
      </div>
    )
  }

  return (
    <div className="space-y-1.5">
      {label ? <FieldLabel>{label}</FieldLabel> : null}
      <AnimatePresence mode="wait">
        <motion.div
          key={type}
          initial={{ opacity: 0, y: 4 }}
          animate={{ opacity: 1, y: 0 }}
          exit={{ opacity: 0, y: -4 }}
          transition={{ duration: 0.15, ease: novaEase }}
        >
          <ValueInputByType
            type={type}
            value={value}
            onChange={onChange}
            options={options}
            min={min}
            max={max}
            compact={compact}
          />
        </motion.div>
      </AnimatePresence>
    </div>
  )
}

function ValueInputByType({
  type,
  value,
  onChange,
  options,
  min,
  max,
  compact,
}: {
  type: string
  value: unknown
  onChange: (value: unknown) => void
  options: string[]
  min?: number
  max?: number
  compact: boolean
}) {
  const { t } = useTranslation()
  // Pre-compute entries for object/list types (hooks can't be in switch)
  const objectEntries = useMemo(() => objectToEntries(value), [value])
  const listEntries = useMemo(() => listToEntries(value), [value])

  if (type === 'number') {
    return (
      <div className="space-y-1">
        <Input
          className={cn('nova-field h-8 text-xs focus-visible:ring-0', compact && 'h-7')}
          inputMode="decimal"
          value={value !== undefined && value !== null ? String(value) : ''}
          onChange={(e) => onChange(parseNumberInput(e.target.value))}
          placeholder={t('settingPanel.actorState.explorer.numberPlaceholder')}
        />
        {min !== undefined || max !== undefined ? (
          <div className="text-[10px] text-[var(--nova-text-faint)]">
            {t('settingPanel.actorState.explorer.range', { min: min ?? '-∞', max: max ?? '+∞' })}
          </div>
        ) : null}
      </div>
    )
  }

  if (type === 'string') {
    return (
      <Input
        className={cn('nova-field h-8 text-xs focus-visible:ring-0', compact && 'h-7')}
        value={String(value ?? '')}
        onChange={(e) => onChange(e.target.value)}
        placeholder={t('settingPanel.actorState.explorer.textPlaceholder')}
      />
    )
  }

  if (type === 'bool') {
    return (
      <div className="flex h-8 items-center gap-2 rounded-[10px] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-3">
        <Switch
          checked={value === true}
          onCheckedChange={onChange}
        />
        <span className="text-xs text-[var(--nova-text-muted)]">
          {value === true ? 'true' : 'false'}
        </span>
      </div>
    )
  }

  if (type === 'enum') {
    if (options.length === 0) {
      return (
        <div className="rounded-[10px] border border-dashed border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-2 text-[10px] text-[var(--nova-text-faint)]">
          {t('settingPanel.actorState.explorer.addEnumFirst')}
        </div>
      )
    }
    return (
      <Select
        value={String(value ?? '')}
        onValueChange={onChange}
      >
        <SelectTrigger className={cn('nova-field h-8 text-xs focus:ring-0', compact && 'h-7')}>
          <SelectValue placeholder={t('settingPanel.actorState.explorer.chooseValue')} />
        </SelectTrigger>
        <SelectContent className="nova-panel border text-[var(--nova-text)]">
          <SelectGroup>
            {options.map((option) => <SelectItem key={option} value={option}>{option}</SelectItem>)}
          </SelectGroup>
        </SelectContent>
      </Select>
    )
  }

  if (type === 'object') {
    return (
      <KeyValEditor
        mode="object"
        entries={objectEntries}
        onChange={(next) => onChange(entriesToObject(next))}
      />
    )
  }

  if (type === 'list') {
    return (
      <KeyValEditor
        mode="list"
        entries={listEntries}
        onChange={(next) => onChange(entriesToList(next))}
      />
    )
  }

  return (
    <Input
      className={cn('nova-field h-8 text-xs focus-visible:ring-0', compact && 'h-7')}
      value={String(value ?? '')}
      onChange={(e) => onChange(e.target.value)}
      placeholder={t('settingPanel.actorState.explorer.valuePlaceholder')}
    />
  )
}

function FieldLabel({ children }: { children: React.ReactNode }) {
  return (
    <span className="block text-[11px] text-[var(--nova-text-faint)]">{children}</span>
  )
}

function getDefaultForType(type: string): unknown {
  switch (type) {
    case 'number': return 0
    case 'string': return ''
    case 'bool': return false
    case 'object': return {}
    case 'list': return []
    default: return ''
  }
}

function objectToEntries(value: unknown): KeyValEntry[] {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return []
  return Object.entries(value as Record<string, unknown>).map(([key, val]) => ({ key, value: val }))
}

function entriesToObject(entries: KeyValEntry[]): Record<string, unknown> {
  const result: Record<string, unknown> = {}
  for (const e of entries) {
    if (e.key) result[e.key] = e.value
  }
  return result
}

function listToEntries(value: unknown): KeyValEntry[] {
  if (!Array.isArray(value)) return []
  return value.map((v) => ({ key: '', value: v }))
}

function entriesToList(entries: KeyValEntry[]): unknown[] {
  return entries.map((e) => e.value)
}
