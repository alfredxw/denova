"use client"

import {
  CircleCheckIcon,
  InfoIcon,
  Loader2Icon,
  OctagonXIcon,
  TriangleAlertIcon,
} from "lucide-react"
import { useTheme } from "next-themes"
import { Toaster as Sonner, type ToasterProps } from "sonner"

const Toaster = ({ ...props }: ToasterProps) => {
  const { theme = "system" } = useTheme()

  return (
    <Sonner
      {...props}
      theme={theme as ToasterProps["theme"]}
      className="toaster group"
      richColors={false}
      icons={{
        success: <CircleCheckIcon className="size-4" />,
        info: <InfoIcon className="size-4" />,
        warning: <TriangleAlertIcon className="size-4" />,
        error: <OctagonXIcon className="size-4" />,
        loading: <Loader2Icon className="size-4 animate-spin" />,
      }}
      toastOptions={{
        classNames: {
          toast: "group toast group-[.toaster]:rounded-[var(--nova-radius)] group-[.toaster]:border group-[.toaster]:border-[var(--nova-border)] group-[.toaster]:bg-[var(--nova-surface)] group-[.toaster]:text-[var(--nova-text)] group-[.toaster]:shadow-[var(--nova-shadow)] group-[.toaster]:backdrop-blur-none",
          title: "group-[.toast]:text-xs group-[.toast]:font-medium group-[.toast]:text-[var(--nova-text)]",
          description: "group-[.toast]:text-xs group-[.toast]:text-[var(--nova-text-muted)]",
          actionButton: "group-[.toast]:rounded-[var(--nova-radius)] group-[.toast]:border group-[.toast]:border-[var(--nova-border)] group-[.toast]:bg-[var(--nova-active)] group-[.toast]:text-[var(--nova-text)]",
          cancelButton: "group-[.toast]:rounded-[var(--nova-radius)] group-[.toast]:border group-[.toast]:border-[var(--nova-border)] group-[.toast]:bg-[var(--nova-surface-2)] group-[.toast]:text-[var(--nova-text-muted)]",
          closeButton: "group-[.toast]:border-[var(--nova-border)] group-[.toast]:bg-[var(--nova-surface-2)] group-[.toast]:text-[var(--nova-text-muted)]",
          error: "group toast group-[.toaster]:border-red-500/30 group-[.toaster]:bg-[var(--nova-surface)] group-[.toaster]:text-[var(--nova-text)] group-[.toaster]:[--normal-border:rgba(239,68,68,0.3)] group-[.toaster]:[--normal-bg:var(--nova-surface)] group-[.toaster]:[--normal-text:var(--nova-text)]",
          success: "group toast group-[.toaster]:border-[var(--nova-border)] group-[.toaster]:bg-[var(--nova-surface)] group-[.toaster]:text-[var(--nova-text)]",
          info: "group toast group-[.toaster]:border-[var(--nova-border)] group-[.toaster]:bg-[var(--nova-surface)] group-[.toaster]:text-[var(--nova-text)]",
          warning: "group toast group-[.toaster]:border-yellow-500/30 group-[.toaster]:bg-[var(--nova-surface)] group-[.toaster]:text-[var(--nova-text)]",
        },
      }}
      style={
        {
          "--normal-bg": "var(--nova-surface)",
          "--normal-text": "var(--nova-text)",
          "--normal-border": "var(--nova-border)",
          "--border-radius": "var(--nova-radius)",
        } as React.CSSProperties
      }
    />
  )
}

export { Toaster }
