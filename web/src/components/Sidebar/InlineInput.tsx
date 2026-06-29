import { useEffect, useRef, useState } from 'react'

interface InlineInputProps {
  defaultValue: string
  /** 是否为重命名模式，会自动选中不含扩展名的部分 */
  isRename?: boolean
  onConfirm: (value: string) => void
  onCancel: () => void
}

/** 目录树内联输入框，用于新建文件/目录和重命名 */
export function InlineInput({ defaultValue, isRename, onConfirm, onCancel }: InlineInputProps) {
  const [value, setValue] = useState(defaultValue)
  const inputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    const el = inputRef.current
    if (!el) return
    el.focus()
    if (isRename) {
      // 选中不含扩展名的部分
      const dotIdx = defaultValue.lastIndexOf('.')
      if (dotIdx > 0) {
        el.setSelectionRange(0, dotIdx)
      } else {
        el.select()
      }
    } else {
      el.select()
    }
  }, [defaultValue, isRename])

  const handleConfirm = () => {
    const trimmed = value.trim()
    if (trimmed && trimmed !== defaultValue) {
      onConfirm(trimmed)
    } else if (trimmed === defaultValue && isRename) {
      // 没改名，取消
      onCancel()
    } else if (trimmed) {
      onConfirm(trimmed)
    } else {
      onCancel()
    }
  }

  return (
    <input
      ref={inputRef}
      value={value}
      onChange={(e) => setValue(e.target.value)}
      onBlur={handleConfirm}
      enterKeyHint="done"
      inputMode="text"
      onKeyDown={(e) => {
        if (e.key === 'Enter') {
          e.preventDefault()
          handleConfirm()
        }
        if (e.key === 'Escape') {
          e.preventDefault()
          onCancel()
        }
        e.stopPropagation()
      }}
      className="h-5 w-full min-w-[80px] rounded border border-[var(--nova-border)] bg-[var(--nova-surface)] px-1 text-xs text-[var(--nova-text)] outline-none max-md:h-8 max-md:min-w-[120px] max-md:px-2 max-md:py-1"
    />
  )
}
