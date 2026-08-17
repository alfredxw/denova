/** Keeps trace summaries faithful to the first meaningful line of model output. */
export function agentContentPreview(content: string) {
  const firstLine = content.split(/\r?\n/).find(line => line.trim())
  return firstLine?.trim().replace(/\s+/g, ' ') || ''
}
