import type { InteractiveImage, InteractiveImageError } from '@/lib/api'

export function readInteractiveImage(result?: string): InteractiveImage | undefined {
  const data = parseEventResult(result)
  if (!data || typeof data !== 'object') return undefined
  const record = data as Record<string, unknown>
  if (record.schema !== 'interactive_image.v1' || typeof record.image_path !== 'string' || !record.image_path) return undefined
  return record as unknown as InteractiveImage
}

export function readInteractiveImageError(result?: string): InteractiveImageError | undefined {
  const data = parseEventResult(result)
  if (!data || typeof data !== 'object') return undefined
  const record = data as Record<string, unknown>
  if (record.schema !== 'interactive_image_error.v1') return undefined
  return record as unknown as InteractiveImageError
}

export function mergeInteractiveImages(persisted?: InteractiveImage[], optimistic?: InteractiveImage[]) {
  const merged: InteractiveImage[] = []
  for (const image of [...(persisted || []), ...(optimistic || [])]) {
    if (!image.image_path || merged.some((item) => item.image_path === image.image_path)) continue
    merged.push(image)
  }
  return merged.length > 0 ? merged : undefined
}

export function latestMergedInteractiveImage(images?: InteractiveImage[]) {
  return images?.[images.length - 1]
}

function parseEventResult(result?: string): unknown {
  if (!result) return null
  try {
    return JSON.parse(result)
  } catch {
    return null
  }
}
