import type { ComponentProps, ReactNode } from 'react'
import { InputArea } from './InputArea'
import { MessageList } from './MessageList'
import { cn } from '@/lib/utils'

interface AgentChatPaneProps {
  emptyContent?: ReactNode
  messageListProps: ComponentProps<typeof MessageList>
  inputAreaProps: ComponentProps<typeof InputArea>
  className?: string
}

/** Shared assembly for the primary chat and the desktop split-chat pane. */
export function AgentChatPane({ emptyContent, messageListProps, inputAreaProps, className }: AgentChatPaneProps) {
  return (
    <div className={cn('relative flex h-full min-h-0 flex-col', className)}>
      {emptyContent}
      <MessageList {...messageListProps} />
      <InputArea {...inputAreaProps} />
    </div>
  )
}
