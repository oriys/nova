import dagre from "dagre"

export interface LayoutPosition {
  x: number
  y: number
}

export type LayoutMap = Record<string, LayoutPosition>

interface LayoutNode {
  key: string
  width: number
  height: number
}

interface LayoutEdge {
  from: string
  to: string
}

/**
 * Compute node positions using dagre auto-layout.
 * If existingLayout is provided and covers all nodes, returns it unchanged.
 */
export function computeLayout(
  nodes: LayoutNode[],
  edges: LayoutEdge[],
  existingLayout?: LayoutMap
): LayoutMap {
  // If we have a complete stored layout, use it
  if (existingLayout) {
    const allPresent = nodes.every((n) => existingLayout[n.key])
    if (allPresent) return existingLayout
  }

  const g = new dagre.graphlib.Graph()
  g.setDefaultEdgeLabel(() => ({}))
  g.setGraph({ rankdir: "TB", ranksep: 80, nodesep: 50, marginx: 40, marginy: 40 })

  for (const node of nodes) {
    g.setNode(node.key, { width: node.width, height: node.height })
  }

  for (const edge of edges) {
    g.setEdge(edge.from, edge.to)
  }

  dagre.layout(g)

  const layout: LayoutMap = {}
  for (const node of nodes) {
    const pos = g.node(node.key)
    if (pos) {
      layout[node.key] = { x: pos.x - node.width / 2, y: pos.y - node.height / 2 }
    }
  }

  return layout
}

/**
 * Validate that a DAG has no cycles using Kahn's algorithm.
 * Returns { valid: true } or { valid: false, message: string }.
 */
export function validateDAG(
  nodeKeys: string[],
  edges: LayoutEdge[]
): { valid: true } | { valid: false; message: string } {
  const inDegree: Record<string, number> = {}
  const adjacency: Record<string, string[]> = {}

  for (const key of nodeKeys) {
    inDegree[key] = 0
    adjacency[key] = []
  }

  for (const edge of edges) {
    if (!adjacency[edge.from]) {
      return { valid: false, message: `Unknown node: ${edge.from}` }
    }
    if (!inDegree.hasOwnProperty(edge.to)) {
      return { valid: false, message: `Unknown node: ${edge.to}` }
    }
    adjacency[edge.from].push(edge.to)
    inDegree[edge.to]++
  }

  const queue: string[] = []
  for (const key of nodeKeys) {
    if (inDegree[key] === 0) queue.push(key)
  }

  let visited = 0
  while (queue.length > 0) {
    const node = queue.shift()!
    visited++
    for (const neighbor of adjacency[node]) {
      inDegree[neighbor]--
      if (inDegree[neighbor] === 0) queue.push(neighbor)
    }
  }

  if (visited !== nodeKeys.length) {
    return { valid: false, message: "Cycle detected in workflow graph" }
  }

  return { valid: true }
}

/** Default node dimensions for layout calculations */
export const NODE_WIDTH = 220
export const NODE_HEIGHT = 80
