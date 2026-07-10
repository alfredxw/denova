import { Plus, Braces, Eye } from 'lucide-react'
import {
  DndContext,
  KeyboardSensor,
  PointerSensor,
  closestCenter,
  useSensor,
  useSensors,
  type DragEndEvent,
} from '@dnd-kit/core'
import { SortableContext, arrayMove, verticalListSortingStrategy } from '@dnd-kit/sortable'
import { AnimatePresence, motion } from 'motion/react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import { novaEase } from '@/features/motion/motion-tokens'
import { cn } from '@/lib/utils'
import { formatPresetJSON } from '../../utils'
import type { StateOp } from '../../../../types'
import { StateOpCard } from './StateOpCard'

interface StateOpBuilderProps {
  ops: StateOp[]
  onChange: (ops: StateOp[]) => void
  pathSuggestions?: string[]
  /** Map of path → field type for value editor type inference */
  pathTypeMap?: Record<string, string>
  /** Map of path → field options (for enum fields) */
  pathOptionsMap?: Record<string, string[]>
}

export function StateOpBuilder({
  ops,
  onChange,
  pathSuggestions = [],
  pathTypeMap = {},
  pathOptionsMap = {},
}: StateOpBuilderProps) {
  const { t } = useTranslation()
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 4 } }),
    useSensor(KeyboardSensor),
  )
  const [viewMode, setViewMode] = useState<'structured' | 'json'>('structured')
  const [jsonText, setJsonText] = useState(() => formatPresetJSON(ops))
  const [jsonError, setJsonError] = useState('')

  const handleDragEnd = (event: DragEndEvent) => {
    const { active, over } = event
    if (!over || active.id === over.id) return
    const oldIndex = Number(String(active.id).replace('op-', ''))
    const newIndex = Number(String(over.id).replace('op-', ''))
    if (isNaN(oldIndex) || isNaN(newIndex)) return
    onChange(arrayMove(ops, oldIndex, newIndex))
  }

  const addOp = () => {
    onChange([
      ...ops,
      { op: 'set', path: '', value: '' },
    ])
  }

  const updateOp = (index: number, newOp: StateOp) => {
    const next = [...ops]
    next[index] = newOp
    onChange(next)
  }

  const removeOp = (index: number) => {
    onChange(ops.filter((_, i) => i !== index))
  }

  const handleJsonChange = (text: string) => {
    setJsonText(text)
    try {
      const parsed = JSON.parse(text)
      if (!Array.isArray(parsed)) throw new Error(t('settingPanel.actorState.explorer.arrayRequired'))
      setJsonError('')
      onChange(parsed)
    } catch (err) {
      setJsonError(err instanceof Error ? err.message : t('settingPanel.actorState.explorer.invalidJSON'))
    }
  }

  // Sync JSON text when ops change externally
  const syncJson = () => {
    setJsonText(formatPresetJSON(ops))
    setJsonError('')
  }

  const sortableIds = ops.map((_, i) => `op-${i}`)

  return (
    <div className="space-y-2">
      {/* Toolbar */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-1">
          <button
            type="button"
            className={cn(
              'flex h-7 items-center gap-1 rounded-full px-2 text-[10px] transition-colors',
              viewMode === 'structured'
                ? 'bg-[var(--nova-text)] text-[var(--nova-surface)]'
                : 'text-[var(--nova-text-faint)] hover:text-[var(--nova-text)]',
            )}
            onClick={() => {
              setViewMode('structured')
              syncJson()
            }}
          >
            <Eye className="h-3 w-3" />
            {t('settingPanel.actorState.explorer.structured')}
          </button>
          <button
            type="button"
            className={cn(
              'flex h-7 items-center gap-1 rounded-full px-2 text-[10px] transition-colors',
              viewMode === 'json'
                ? 'bg-[var(--nova-text)] text-[var(--nova-surface)]'
                : 'text-[var(--nova-text-faint)] hover:text-[var(--nova-text)]',
            )}
            onClick={() => {
              setViewMode('json')
              syncJson()
            }}
          >
            <Braces className="h-3 w-3" />
            JSON
          </button>
        </div>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          className="h-7 rounded-full px-2.5 text-[11px] text-[var(--nova-text-faint)] hover:text-[var(--nova-text)]"
          onClick={addOp}
        >
          <Plus data-icon="inline-start" />
          {t('settingPanel.actorState.explorer.addOperation')}
        </Button>
      </div>

      {/* Content */}
      <AnimatePresence mode="wait">
        {viewMode === 'structured' ? (
          <motion.div
            key="structured"
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
            transition={{ duration: 0.15, ease: novaEase }}
          >
            {ops.length === 0 ? (
              <div className="rounded-[12px] border border-dashed border-[var(--nova-border)] bg-[var(--nova-surface)] px-4 py-8 text-center text-[11px] text-[var(--nova-text-faint)]">
                {t('settingPanel.actorState.explorer.emptyOperations')}
              </div>
            ) : (
              <DndContext
                sensors={sensors}
                collisionDetection={closestCenter}
                onDragEnd={handleDragEnd}
              >
                <SortableContext items={sortableIds} strategy={verticalListSortingStrategy}>
                  <div className="space-y-2">
                    <AnimatePresence initial={false}>
                      {ops.map((op, i) => (
                        <StateOpCard
                          key={`op-${i}`}
                          op={op}
                          index={i}
                          onChange={(newOp) => updateOp(i, newOp)}
                          onRemove={() => removeOp(i)}
                          pathSuggestions={pathSuggestions}
                          valueType={pathTypeMap[op.path]}
                          valueOptions={pathOptionsMap[op.path]}
                        />
                      ))}
                    </AnimatePresence>
                  </div>
                </SortableContext>
              </DndContext>
            )}
          </motion.div>
        ) : (
          <motion.div
            key="json"
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
            transition={{ duration: 0.15, ease: novaEase }}
            className="space-y-1.5"
          >
            <Textarea
              className="nova-field min-h-48 resize-y font-mono text-xs leading-5 shadow-none focus-visible:ring-0"
              value={jsonText}
              onChange={(e) => handleJsonChange(e.target.value)}
            />
            {jsonError ? (
              <div className="rounded-[var(--nova-radius)] border border-[var(--nova-danger-border)] bg-[var(--nova-danger-bg)] px-2 py-1 text-[11px] text-[var(--nova-danger)]">
                {jsonError}
              </div>
            ) : null}
          </motion.div>
        )}
      </AnimatePresence>
    </div>
  )
}
