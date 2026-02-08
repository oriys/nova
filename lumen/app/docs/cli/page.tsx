import { DocsShell } from "@/components/docs/docs-shell"
import { CodeBlock } from "@/components/docs/code-block"

export default function DocsCLIPage() {
  return (
    <DocsShell
      current="cli"
      title="Orbit CLI"
      description="Orbit is the primary CLI for Nova operations. Nova binary now runs backend services only (daemon mode)."
      toc={[
        { id: "positioning", label: "Positioning" },
        { id: "global-context", label: "Global Context" },
        { id: "command-groups", label: "Command Groups" },
        { id: "task-recipes", label: "Task Recipes" },
        { id: "pull-local-test", label: "Pull & Local Test" },
        { id: "troubleshooting", label: "Troubleshooting" },
      ]}
    >
      <section id="positioning" className="scroll-mt-24">
        <h2 className="text-3xl font-semibold tracking-tight">Positioning</h2>
        <ul className="mt-4 list-disc space-y-3 pl-6 text-lg leading-8">
          <li>
            <strong>Nova binary:</strong> backend service process only, run with <code>nova daemon</code>.
          </li>
          <li>
            <strong>Orbit CLI:</strong> full operational surface for functions, workflows, events, tenants, secrets,
            metrics, and gateway.
          </li>
        </ul>
        <CodeBlock
          code={`# Nova backend service
./bin/nova daemon --config ./configs/nova.yaml

# Orbit operator CLI
orbit --help
orbit functions --help`}
        />
      </section>

      <section id="global-context" className="scroll-mt-24">
        <h2 className="text-3xl font-semibold tracking-tight">Global Context</h2>
        <p className="mt-4 text-lg leading-8 text-muted-foreground">
          Orbit resolves target server and tenant scope from flags, environment variables, or local config.
        </p>
        <CodeBlock
          code={`# Flags
--server      Nova API URL
--api-key     API key
--tenant      Tenant ID
--namespace   Namespace
--output      table|wide|json|yaml

# Environment variables
NOVA_URL
NOVA_API_KEY
NOVA_TENANT
NOVA_NAMESPACE
NOVA_OUTPUT

# Persist defaults
orbit config set server http://localhost:9000
orbit config set tenant default
orbit config set namespace default
orbit config set output table`}
        />
      </section>

      <section id="command-groups" className="scroll-mt-24">
        <h2 className="text-3xl font-semibold tracking-tight">Command Groups</h2>
        <CodeBlock
          code={`Core
- orbit functions (alias: fn)
- orbit workflows (alias: wf)
- orbit topics | subscriptions | deliveries

Platform
- orbit tenants
- orbit runtimes
- orbit gateway (alias: gw)
- orbit layers
- orbit apikeys
- orbit secrets

Operations
- orbit metrics
- orbit health
- orbit stats
- orbit invocations
- orbit async-invocations
- orbit config`}
        />
      </section>

      <section id="task-recipes" className="scroll-mt-24">
        <h2 className="text-3xl font-semibold tracking-tight">Task Recipes</h2>
        <p className="mt-4 text-lg leading-8 text-muted-foreground">
          Use these as copy-paste starters for common operator workflows.
        </p>
        <CodeBlock
          code={`# 1) Function lifecycle
orbit functions create \
  --name echo \
  --runtime python \
  --handler handler \
  --code 'def handler(event, context):\n    return event' \
  --memory 256 \
  --timeout 30 \
  --min-replicas 1 \
  --max-replicas 10 \
  --instance-concurrency 10

orbit functions invoke echo --payload '{"hello":"nova"}'
orbit functions update echo --memory 512 --timeout 60
orbit functions logs echo --tail 20

# 2) Workflow and run
orbit workflows create --name order-pipeline --description "Order processing"
orbit workflows run order-pipeline --input '{"order_id":"o-1"}'
orbit workflows runs order-pipeline

# 3) Eventing
orbit topics create --name orders
orbit topics subscriptions create \
  orders \
  --name orders-to-echo \
  --function echo
orbit topics publish orders --payload '{"order_id":"o-1"}'
orbit topics subscriptions list orders`}
        />
      </section>

      <section id="pull-local-test" className="scroll-mt-24">
        <h2 className="text-3xl font-semibold tracking-tight">Pull &amp; Local Test</h2>
        <p className="mt-4 text-lg leading-8 text-muted-foreground">
          Orbit can pull remote function source to local workspace and run a local test with your payload.
          If runtime toolchain is missing, Orbit prints install instructions directly.
        </p>
        <CodeBlock
          code={`# Pull remote function source to local folder
orbit functions pull echo --output-dir ./local-fns

# Pull and run local test immediately
orbit functions pull echo \\
  --output-dir ./local-fns \\
  --test \\
  --payload '{"hello":"local"}'

# Use payload file and overwrite existing local folder
orbit functions pull echo \\
  --output-dir ./local-fns \\
  --force \\
  --test \\
  --payload-file ./payload.json`}
        />
        <ul className="mt-4 list-disc space-y-3 pl-6 text-lg leading-8">
          <li>Auto local runner currently supports Python and Node.js runtimes.</li>
          <li>Go/Rust/Java runtimes still pull source and check toolchain, then show manual test guidance.</li>
          <li>Pulled files include source code, <code>payload.json</code>, and <code>function.meta.json</code>.</li>
        </ul>
      </section>

      <section id="troubleshooting" className="scroll-mt-24">
        <h2 className="text-3xl font-semibold tracking-tight">Troubleshooting</h2>
        <ul className="mt-4 list-disc space-y-3 pl-6 text-lg leading-8">
          <li>If commands fail with scope errors, set <code>--tenant</code> and <code>--namespace</code> explicitly.</li>
          <li>If auth is enabled, verify <code>--api-key</code> or Authorization header configuration.</li>
          <li>If create/invoke succeeds but metrics look empty, check the selected tenant badge in Lumen.</li>
          <li>For noisy output in automation, use <code>--output json</code> and parse deterministically.</li>
        </ul>
      </section>
    </DocsShell>
  )
}
