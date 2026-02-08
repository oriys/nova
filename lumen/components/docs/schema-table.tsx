export interface SchemaField {
  name: string
  type: string
  required?: boolean
  description: string
}

interface SchemaTableProps {
  title: string
  fields: SchemaField[]
}

export function SchemaTable({ title, fields }: SchemaTableProps) {
  return (
    <div className="space-y-2">
      <p className="text-sm font-medium text-foreground">{title}</p>
      <div className="overflow-x-auto rounded-lg border border-border">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-border bg-muted/30">
              <th className="px-3 py-2 text-left font-medium text-muted-foreground">Field</th>
              <th className="px-3 py-2 text-left font-medium text-muted-foreground">Type</th>
              <th className="px-3 py-2 text-left font-medium text-muted-foreground">Required</th>
              <th className="px-3 py-2 text-left font-medium text-muted-foreground">Description</th>
            </tr>
          </thead>
          <tbody>
            {fields.map((field) => (
              <tr key={field.name} className="border-b border-border last:border-0">
                <td className="px-3 py-2 font-mono text-xs text-foreground">{field.name}</td>
                <td className="px-3 py-2 text-muted-foreground">{field.type}</td>
                <td className="px-3 py-2 text-muted-foreground">{field.required ? "yes" : "no"}</td>
                <td className="px-3 py-2 text-muted-foreground">{field.description}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}
