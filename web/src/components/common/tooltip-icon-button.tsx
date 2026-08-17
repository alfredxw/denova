import type { ReactNode } from 'react'
import { Button, type buttonVariants } from '@/components/ui/button'
import {
  DEFAULT_TOOLTIP_DELAY_MS,
  DEFAULT_TOOLTIP_SKIP_DELAY_MS,
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import type { VariantProps } from 'class-variance-authority'

type TooltipIconButtonProps = React.ComponentProps<'button'> &
  VariantProps<typeof buttonVariants> & {
    label: string
    children: ReactNode
    showTooltip?: boolean
    tooltipDelayMs?: number
    tooltipSkipDelayMs?: number
    tooltipSide?: 'top' | 'right' | 'bottom' | 'left'
    tooltipSideOffset?: number
    useTooltipProvider?: boolean
  }

/** Icon button with shared accessible naming and tooltip timing. */
export function TooltipIconButton({
  label,
  children,
  variant = 'ghost',
  size = 'icon-xs',
  className,
  showTooltip = true,
  tooltipDelayMs = DEFAULT_TOOLTIP_DELAY_MS,
  tooltipSkipDelayMs = DEFAULT_TOOLTIP_SKIP_DELAY_MS,
  tooltipSide = 'right',
  tooltipSideOffset = 6,
  useTooltipProvider = true,
  'aria-label': ariaLabel,
  ...props
}: TooltipIconButtonProps) {
  const button = (
    <Button
      type="button"
      variant={variant}
      size={size}
      className={className}
      aria-label={ariaLabel || label}
      {...props}
    >
      {children}
    </Button>
  )

  if (!showTooltip) return button

  const tooltip = (
    <Tooltip>
      <TooltipTrigger asChild>{button}</TooltipTrigger>
      <TooltipContent side={tooltipSide} sideOffset={tooltipSideOffset}>{label}</TooltipContent>
    </Tooltip>
  )

  if (!useTooltipProvider) return tooltip

  return (
    <TooltipProvider delayDuration={tooltipDelayMs} skipDelayDuration={tooltipSkipDelayMs}>
      {tooltip}
    </TooltipProvider>
  )
}
