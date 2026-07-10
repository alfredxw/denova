import { ChevronDown, ChevronRight, Combine, GripVertical, ListMinus, ListPlus, Pencil, Sigma, Trash2, type LucideIcon } from 'lucide-react'
import { AnimatePresence, motion } from 'motion/react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useSortable } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import { novaEase } from '@/features/motion/motion-tokens'
import { cn } from '@/lib/utils'
import type { StateOp } from '../../../../types'
import { StateValueEditor } from '../shared/StateValueEditor'

const OP_TYPES = [
  { value: 'set', labelKey: 'settingPanel.actorState.explorer.op.set', icon: Pencil },
  { value: 'merge', labelKey: 'settingPanel.actorState.explorer.op.merge', icon: Combine },
  { value: 'push', labelKey: 'settingPanel.actorState.explorer.op.push', icon: ListPlus },
  { value: 'pull', labelKey: 'settingPanel.actorState.explorer.op.pull', icon: ListMinus },
  { value: 'inc', labelKey: 'settingPanel.actorState.explorer.op.inc', icon: Sigma },
  { value: 'unset', labelKey: 'settingPanel.actorState.explorer.op.unset', icon: Trash2 },
] satisfies Array<{ value: string; labelKey: string; icon: LucideIcon }>

interface StateOpCardProps {
  op: StateOp
  index: number
  onChange: (op: StateOp) => void
  onRemove: () => void
  pathSuggestions?: string[]
  valueType?: string
  valueOptions?: string[]
}

export function StateOpCard({
  op,
  index,
  onChange,
  onRemove,
  pathSuggestions = [],
  valueType,
  valueOptions,
}: StateOpCardProps) {
  const { t } = useTranslation()
  const [showReason, setShowReason] = useState(false)
  const [showSuggestions, setShowSuggestions] = useState(false)
  const [pathFilter, setPathFilter] = useState('')

  const {
    attributes,
    listeners,
    setNodeRef,
    setActivatorNodeRef,
    transform,
    transition,
    isDragging,
  } = useSortable({ id: `op-${index}` })

  const style = {
    transform: CSS.Transform.toString(transform),
    transition,
  }

  const needsValue = op.op !== 'unset'

  // Determine the effective value type for the editor
  const effectiveType = valueType || inferValueType(op)

  const filteredSuggestions = pathSuggestions.filter(
    (s) => !pathFilter || s.toLowerCase().includes(pathFilter.toLowerCase()),
  )

  const handlePathFocus = () => {
    if (pathSuggestions.length > 0) {
      setShowSuggestions(true)
      setPathFilter(op.path)
    }
  }

  const handlePathChange = (value: string) => {
    onChange({ ...op, path: value })
    setPathFilter(value)
  }

  const selectSuggestion = (suggestion: string) => {
    onChange({ ...op, path: suggestion })
    setShowSuggestions(false)
  }

  return (
    <motion.div
      ref={setNodeRef}
      style={style}
      layout
      initial={{ opacity: 0, y: 5 }}
      animate={{ opacity: 1, y: 0 }}
      exit={{ opacity: 0, y: -4 }}
      transition={{ duration: 0.15, ease: novaEase }}
      className={cn(
        'group relative rounded-[14px] border bg-[var(--nova-surface)] p-3',
        isDragging ? 'opacity-50 z-10' : '',
      )}
    >
      <div className="flex items-start gap-2">
        {/* Drag handle */}
        <button
          ref={setActivatorNodeRef}
          type="button"
          className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-[8px] text-[var(--nova-text-faint)] opacity-0 transition-opacity hover:bg-[var(--nova-hover)] group-hover:opacity-100 group-focus-within:opacity-100 focus-visible:opacity-100 [@media(pointer:coarse)]:opacity-100"
          aria-label={t('settingPanel.actorState.explorer.drag')}
          {...attributes}
          {...listeners}
        >
          <GripVertical className="h-3.5 w-3.5" />
        </button>

        {/* Op type selector */}
        <div className="shrink-0">
          <Select
            value={op.op}
            onValueChange={(v) => onChange({ ...op, op: v })}
          >
            <SelectTrigger className="nova-field h-8 w-[90px] text-xs focus:ring-0">
              <SelectValue />
            </SelectTrigger>
            <SelectContent className="nova-panel border text-[var(--nova-text)]">
              <SelectGroup>
                {OP_TYPES.map((item) => {
                  const Icon = item.icon
                  return (
                    <SelectItem key={item.value} value={item.value}>
                      <Icon className="text-[var(--nova-text-faint)]" />
                      {t(item.labelKey)}
                    </SelectItem>
                  )
                })}
              </SelectGroup>
            </SelectContent>
          </Select>
        </div>

        {/* Path input */}
        <div className="relative min-w-0 flex-1">
          <Input
            className="nova-field h-8 font-mono text-xs focus-visible:ring-0"
            value={op.path}
            onChange={(e) => handlePathChange(e.target.value)}
            onFocus={handlePathFocus}
            onBlur={() => setTimeout(() => setShowSuggestions(false), 150)}
            placeholder="actors.protagonist.state.health"
          />
          {/* Path suggestions dropdown */}
          <AnimatePresence>
            {showSuggestions && filteredSuggestions.length > 0 ? (
              <motion.div
                initial={{ opacity: 0, y: -4 }}
                animate={{ opacity: 1, y: 0 }}
                exit={{ opacity: 0, y: -4 }}
                transition={{ duration: 0.1 }}
                className="absolute left-0 right-0 top-full z-20 mt-1 max-h-48 overflow-y-auto rounded-[10px] border border-[var(--nova-border)] bg-[var(--nova-surface)] py-1 shadow-lg"
              >
                {filteredSuggestions.slice(0, 20).map((s) => (
                  <button
                    key={s}
                    type="button"
                    className="block w-full truncate px-3 py-1.5 text-left font-mono text-[11px] text-[var(--nova-text-muted)] transition-colors hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]"
                    onMouseDown={(e) => {
                      e.preventDefault()
                      selectSuggestion(s)
                    }}
                  >
                    {s}
                  </button>
                ))}
              </motion.div>
            ) : null}
          </AnimatePresence>
        </div>

        {/* Delete button */}
        <button
          type="button"
          className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-full text-[var(--nova-text-faint)] opacity-0 transition-opacity hover:bg-[var(--nova-danger-bg)] hover:text-[var(--nova-danger)] group-hover:opacity-100 group-focus-within:opacity-100 focus-visible:opacity-100 [@media(pointer:coarse)]:opacity-100"
          onClick={onRemove}
          aria-label={t('settingPanel.actorState.explorer.deleteOperation')}
        >
          <Trash2 className="h-3.5 w-3.5" />
        </button>
      </div>

      {/* Value editor */}
      <AnimatePresence>
        {needsValue ? (
          <motion.div
            initial={{ height: 0, opacity: 0 }}
            animate={{ height: 'auto', opacity: 1 }}
            exit={{ height: 0, opacity: 0 }}
            transition={{ duration: 0.15, ease: novaEase }}
            className="overflow-hidden"
          >
            <div className="mt-2 pl-7">
              <StateValueEditor
                type={effectiveType}
                value={op.value}
                onChange={(v) => onChange({ ...op, value: v })}
                options={valueOptions}
                compact
              />
            </div>
          </motion.div>
        ) : null}
      </AnimatePresence>

      {/* Reason */}
      <div className="mt-1 pl-7">
        <button
          type="button"
          className="flex min-h-7 items-center gap-1 rounded-[8px] px-1 text-[10px] text-[var(--nova-text-faint)] transition-colors hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]"
          onClick={() => setShowReason(!showReason)}
          aria-expanded={showReason}
        >
          {showReason ? (
            <ChevronDown className="h-3 w-3" />
          ) : (
            <ChevronRight className="h-3 w-3" />
          )}
          <span>{op.reason ? t('settingPanel.actorState.explorer.reasonSet') : t('settingPanel.actorState.explorer.reason')}</span>
        </button>
        <AnimatePresence>
          {showReason ? (
            <motion.div
              initial={{ height: 0, opacity: 0 }}
              animate={{ height: 'auto', opacity: 1 }}
              exit={{ height: 0, opacity: 0 }}
              transition={{ duration: 0.15, ease: novaEase }}
              className="overflow-hidden"
            >
              <Textarea
                className="nova-field mt-1 min-h-[40px] resize-none text-xs focus-visible:ring-0"
                value={op.reason || ''}
                onChange={(e) => onChange({ ...op, reason: e.target.value })}
                placeholder={t('settingPanel.actorState.explorer.reasonPlaceholder')}
              />
            </motion.div>
          ) : null}
        </AnimatePresence>
      </div>

      {/* Op type indicator line */}
      <div
        className={cn(
          'pointer-events-none absolute left-0 top-3 h-[calc(100%-1.5rem)] w-[3px] rounded-r-full',
          getOpColorClass(op.op),
        )}
      />
    </motion.div>
  )
}

function inferValueType(op: StateOp): string {
  if (op.op === 'inc') return 'number'
  if (op.op === 'merge') return 'object'
  if (op.op === 'push' || op.op === 'pull') return 'list'
  return 'string'
}

function getOpColorClass(op: string): string {
  return op === 'unset' ? 'bg-[var(--nova-danger)]/60' : 'bg-[var(--nova-accent)]/60'
}
