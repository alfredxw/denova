import { AnimatePresence, motion } from 'motion/react'
import { GripVertical, Plus, Trash2 } from 'lucide-react'
import { useSortable } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import {
  DndContext,
  DragOverlay,
  KeyboardSensor,
  PointerSensor,
  closestCenter,
  useSensor,
  useSensors,
  type DragEndEvent,
} from '@dnd-kit/core'
import { SortableContext, arrayMove, verticalListSortingStrategy } from '@dnd-kit/sortable'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { novaEase } from '@/features/motion/motion-tokens'
import { cn } from '@/lib/utils'

export interface KeyValEntry {
  key: string
  value: unknown
}

interface KeyValEditorProps {
  entries: KeyValEntry[]
  onChange: (entries: KeyValEntry[]) => void
  mode: 'object' | 'list'
  placeholder?: string
}

export function KeyValEditor({ entries, onChange, mode, placeholder }: KeyValEditorProps) {
  const { t } = useTranslation()
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 4 } }),
    useSensor(KeyboardSensor),
  )
  const [draggingId, setDraggingId] = useState<string | null>(null)

  const ids = entries.map((_, i) => `kv-${i}`)

  const handleDragEnd = (event: DragEndEvent) => {
    setDraggingId(null)
    const { active, over } = event
    if (!over || active.id === over.id) return
    const oldIndex = ids.indexOf(String(active.id))
    const newIndex = ids.indexOf(String(over.id))
    if (oldIndex < 0 || newIndex < 0) return
    onChange(arrayMove(entries, oldIndex, newIndex))
  }

  const updateEntry = (index: number, patch: Partial<KeyValEntry>) => {
    onChange(entries.map((e, i) => (i === index ? { ...e, ...patch } : e)))
  }

  const removeEntry = (index: number) => {
    onChange(entries.filter((_, i) => i !== index))
  }

  const addEntry = () => {
    if (mode === 'list') {
      onChange([...entries, { key: '', value: '' }])
    } else {
      onChange([...entries, { key: `field_${entries.length}`, value: '' }])
    }
  }

  return (
    <div className="space-y-1.5">
      <DndContext
        sensors={sensors}
        collisionDetection={closestCenter}
        onDragStart={(e) => setDraggingId(String(e.active.id))}
        onDragCancel={() => setDraggingId(null)}
        onDragEnd={handleDragEnd}
      >
        <SortableContext items={ids} strategy={verticalListSortingStrategy}>
          <AnimatePresence initial={false}>
            {entries.map((entry, index) => (
              <KeyValRow
                key={`kv-${index}`}
                id={`kv-${index}`}
                entry={entry}
                index={index}
                mode={mode}
                placeholder={placeholder}
                onChange={(patch) => updateEntry(index, patch)}
                onRemove={() => removeEntry(index)}
              />
            ))}
          </AnimatePresence>
        </SortableContext>
        <DragOverlay>
          {draggingId ? (
            <div className="rounded-[10px] bg-[var(--nova-surface)] px-2 py-1.5 text-xs text-[var(--nova-text)] shadow-[0_8px_24px_rgba(0,0,0,0.3)] ring-1 ring-[var(--nova-accent)]/25">
              {t('settingPanel.actorState.explorer.dragging')}
            </div>
          ) : null}
        </DragOverlay>
      </DndContext>
      <Button
        type="button"
        variant="ghost"
        size="sm"
        className="h-7 w-full rounded-[10px] border border-dashed border-[var(--nova-border)] text-[var(--nova-text-faint)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]"
        onClick={addEntry}
      >
        <Plus data-icon="inline-start" />
        {mode === 'list' ? t('settingPanel.actorState.explorer.addItem') : t('settingPanel.actorState.addField')}
      </Button>
    </div>
  )
}

interface KeyValRowProps {
  id: string
  entry: KeyValEntry
  index: number
  mode: 'object' | 'list'
  placeholder?: string
  onChange: (patch: Partial<KeyValEntry>) => void
  onRemove: () => void
}

function KeyValRow({ id, entry, mode, placeholder, onChange, onRemove }: KeyValRowProps) {
  const { t } = useTranslation()
  const { attributes, listeners, setNodeRef, setActivatorNodeRef, transform, transition, isDragging } = useSortable({ id })

  const style = {
    transform: CSS.Transform.toString(transform),
    transition,
  }

  return (
    <motion.div
      ref={setNodeRef}
      style={style}
      initial={{ opacity: 0, y: -4 }}
      animate={{ opacity: 1, y: 0 }}
      exit={{ opacity: 0, y: -4 }}
      transition={{ duration: 0.15, ease: novaEase }}
      className={cn(
        'group flex items-center gap-1.5 rounded-[10px] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-1.5 py-1',
        isDragging && 'opacity-50',
      )}
    >
      <button
        ref={setActivatorNodeRef}
        type="button"
        className="flex size-8 shrink-0 items-center justify-center rounded-[8px] text-[var(--nova-text-faint)] opacity-0 transition-opacity hover:bg-[var(--nova-hover)] group-hover:opacity-100 group-focus-within:opacity-100 focus-visible:opacity-100 [@media(pointer:coarse)]:opacity-100"
        aria-label={t('settingPanel.actorState.explorer.drag')}
        {...attributes}
        {...listeners}
      >
        <GripVertical className="h-3.5 w-3.5" />
      </button>

      {mode === 'object' ? (
        <Input
          className="h-7 min-w-0 flex-1 border-0 bg-transparent px-1 text-xs font-mono focus-visible:ring-0 focus-visible:ring-offset-0"
          value={entry.key}
          onChange={(e) => onChange({ key: e.target.value })}
          placeholder={t('settingPanel.actorState.explorer.keyPlaceholder')}
        />
      ) : null}

      <Input
        className={cn(
          'h-7 min-w-0 flex-1 border-0 bg-transparent px-1 text-xs focus-visible:ring-0 focus-visible:ring-offset-0',
          mode === 'list' && 'flex-[2]',
        )}
        value={String(entry.value ?? '')}
        onChange={(e) => onChange({ value: e.target.value })}
        placeholder={placeholder || t('settingPanel.actorState.explorer.valuePlaceholder')}
      />

      <button
        type="button"
        className="flex size-8 shrink-0 items-center justify-center rounded-full text-[var(--nova-text-faint)] opacity-0 transition-opacity hover:bg-[var(--nova-hover)] hover:text-[var(--nova-danger)] group-hover:opacity-100 group-focus-within:opacity-100 focus-visible:opacity-100 [@media(pointer:coarse)]:opacity-100"
        onClick={onRemove}
        aria-label={t('common.delete')}
      >
        <Trash2 className="h-3.5 w-3.5" />
      </button>
    </motion.div>
  )
}
