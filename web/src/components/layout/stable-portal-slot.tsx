import { useLayoutEffect, useRef, type HTMLAttributes, type ReactNode } from 'react'

export function createStablePortalHost(className: string): HTMLDivElement | null {
  if (typeof document === 'undefined') return null
  const host = document.createElement('div')
  host.className = className
  return host
}

interface StablePortalSlotProps extends HTMLAttributes<HTMLDivElement> {
  host: HTMLDivElement | null
  fallback: ReactNode
  wrapFallback?: boolean
}

/** Keeps a mounted subtree stable while its host moves between responsive layouts. */
export function StablePortalSlot({ host, fallback, wrapFallback = true, ...props }: StablePortalSlotProps) {
  const slotRef = useRef<HTMLDivElement>(null)

  useLayoutEffect(() => {
    const slot = slotRef.current
    if (!host || !slot) return
    slot.appendChild(host)
    return () => {
      if (host.parentNode === slot) host.remove()
    }
  }, [host])

  if (!host) return wrapFallback ? <div {...props}>{fallback}</div> : fallback
  return <div ref={slotRef} {...props} />
}
