import type { FileNode } from '@/hooks/useWorkspace'

export function flattenFileTree(nodes: FileNode[], basePath = ''): string[] {
  return nodes.flatMap((node) => {
    const path = basePath ? `${basePath}/${node.name}` : node.name
    if (node.type === 'file') return [path]
    return flattenFileTree(node.children || [], path)
  })
}

export function formatNumber(value: number) {
  return new Intl.NumberFormat('zh-CN').format(value)
}
