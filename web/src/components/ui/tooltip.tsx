"use client"

import * as React from "react"
import { Tooltip as TooltipPrimitive } from "radix-ui"

import { cn } from "@/lib/utils"

/** Keeps incidental pointer movement from producing tooltip noise. */
const DEFAULT_TOOLTIP_DELAY_MS = 1000
/** Requires a fresh dwell for every trigger instead of opening adjacent hints immediately. */
const DEFAULT_TOOLTIP_SKIP_DELAY_MS = 0

function TooltipProvider({
  delayDuration = DEFAULT_TOOLTIP_DELAY_MS,
  skipDelayDuration = DEFAULT_TOOLTIP_SKIP_DELAY_MS,
  ...props
}: React.ComponentProps<typeof TooltipPrimitive.Provider>) {
  return (
    <TooltipPrimitive.Provider
      data-slot="tooltip-provider"
      delayDuration={delayDuration}
      skipDelayDuration={skipDelayDuration}
      {...props}
    />
  )
}

function Tooltip({
  ...props
}: React.ComponentProps<typeof TooltipPrimitive.Root>) {
  return <TooltipPrimitive.Root data-slot="tooltip" {...props} />
}

function TooltipTrigger({
  ...props
}: React.ComponentProps<typeof TooltipPrimitive.Trigger>) {
  return <TooltipPrimitive.Trigger data-slot="tooltip-trigger" {...props} />
}

function TooltipContent({
  className,
  sideOffset = 6,
  children,
  ...props
}: React.ComponentProps<typeof TooltipPrimitive.Content>) {
  return (
    <TooltipPrimitive.Portal>
      <TooltipPrimitive.Content
        data-slot="tooltip-content"
        sideOffset={sideOffset}
        className={cn(
          "pointer-events-none z-50 inline-flex w-fit max-w-xs origin-(--radix-tooltip-content-transform-origin) items-center gap-1.5 rounded-md border border-[var(--nova-border)] [--tooltip-bg:color-mix(in_srgb,var(--nova-surface-3)_94%,var(--nova-text)_6%)] bg-[var(--tooltip-bg)] px-2.5 py-1.5 text-left text-[11px] leading-4 break-words whitespace-pre-wrap text-[var(--nova-text)] shadow-[0_10px_28px_rgba(0,0,0,0.22)] has-data-[slot=kbd]:pr-1.5 data-[side=bottom]:slide-in-from-top-1 data-[side=left]:slide-in-from-right-1 data-[side=right]:slide-in-from-left-1 data-[side=top]:slide-in-from-bottom-1 **:data-[slot=kbd]:relative **:data-[slot=kbd]:isolate **:data-[slot=kbd]:z-50 **:data-[slot=kbd]:rounded-sm data-[state=delayed-open]:animate-in data-[state=delayed-open]:fade-in-0 data-[state=delayed-open]:zoom-in-95 data-open:animate-in data-open:fade-in-0 data-open:zoom-in-95 data-closed:animate-out data-closed:fade-out-0 data-closed:zoom-out-95",
          className
        )}
        {...props}
      >
        {children}
        <TooltipPrimitive.Arrow className="z-50 size-2.5 translate-y-[calc(-50%_-_2px)] rotate-45 rounded-[2px] bg-[var(--tooltip-bg)] fill-[var(--tooltip-bg)]" />
      </TooltipPrimitive.Content>
    </TooltipPrimitive.Portal>
  )
}

export {
  DEFAULT_TOOLTIP_DELAY_MS,
  DEFAULT_TOOLTIP_SKIP_DELAY_MS,
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
}
