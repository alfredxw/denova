import type { Modifiers } from '@dnd-kit/core'

export const verticalAxisModifiers: Modifiers = [
  ({ transform }) => ({ ...transform, x: 0 }),
]
