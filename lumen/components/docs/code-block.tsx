import { cn } from "@/lib/utils"

interface CodeBlockProps {
  code: string
  className?: string
}

export function CodeBlock({ code, className }: CodeBlockProps) {
  return (
    <pre
      className={cn(
        "overflow-x-auto rounded-xl border border-border bg-muted/30 p-4 text-xs leading-6 text-foreground",
        className
      )}
    >
      <code>{code}</code>
    </pre>
  )
}
