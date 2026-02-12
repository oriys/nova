// Runtime logos using devicon.dev icons

// Devicon class name mapping for each runtime
const DEVICON_CLASSES: Record<string, string> = {
  python: "devicon-python-plain",
  go: "devicon-go-plain",
  node: "devicon-nodejs-plain",
  rust: "devicon-rust-original",
  ruby: "devicon-ruby-plain",
  java: "devicon-java-plain",
  php: "devicon-php-plain",
  deno: "devicon-denojs-original",
  bun: "devicon-bun-plain",
  swift: "devicon-swift-plain",
  elixir: "devicon-elixir-plain",
  kotlin: "devicon-kotlin-plain",
  scala: "devicon-scala-plain",
  zig: "devicon-zig-plain",
  lua: "devicon-lua-plain",
  perl: "devicon-perl-plain",
  r: "devicon-r-plain",
  julia: "devicon-julia-plain",
  wasm: "devicon-wasm-original",
  typescript: "devicon-typescript-plain",
  javascript: "devicon-javascript-plain",
  c: "devicon-c-plain",
  cplusplus: "devicon-cplusplus-plain",
  csharp: "devicon-csharp-plain",
  haskell: "devicon-haskell-plain",
  clojure: "devicon-clojure-plain",
  erlang: "devicon-erlang-plain",
  ocaml: "devicon-ocaml-plain",
}

// Runtime brand colors
export const RUNTIME_COLORS: Record<string, string> = {
  python: "bg-[#3776AB]",
  go: "bg-[#00ADD8]",
  node: "bg-[#339933]",
  rust: "bg-[#DEA584]",
  ruby: "bg-[#CC342D]",
  java: "bg-[#ED8B00]",
  php: "bg-[#777BB4]",
  deno: "bg-[#000000]",
  bun: "bg-[#FBF0DF]",
  swift: "bg-[#F05138]",
  elixir: "bg-[#4B275F]",
  kotlin: "bg-[#7F52FF]",
  scala: "bg-[#DC322F]",
  zig: "bg-[#F7A41D]",
  lua: "bg-[#000080]",
  perl: "bg-[#39457E]",
  r: "bg-[#276DC3]",
  julia: "bg-[#9558B2]",
  wasm: "bg-[#654FF0]",
}

// Get base runtime from versioned ID (e.g., "python3.11" -> "python")
function getBaseRuntime(runtimeId: string): string {
  const prefixes = ['python', 'go', 'node', 'rust', 'ruby', 'java', 'php', 'scala']
  for (const prefix of prefixes) {
    if (runtimeId.startsWith(prefix)) return prefix
  }
  return runtimeId
}

export function getDeviconClass(runtimeId: string): string | null {
  const baseRuntime = getBaseRuntime(runtimeId)
  return DEVICON_CLASSES[baseRuntime] || null
}

export function getRuntimeColor(runtimeId: string): string {
  const baseRuntime = getBaseRuntime(runtimeId)
  return RUNTIME_COLORS[baseRuntime] || "bg-gray-500"
}

// Component that renders a devicon
export function RuntimeIcon({
  runtimeId,
  className = ""
}: {
  runtimeId: string
  className?: string
}) {
  const iconClass = getDeviconClass(runtimeId)

  if (!iconClass) {
    return <span className={className}>?</span>
  }

  return <i className={`${iconClass} ${className}`} />
}
