import { getTranslations } from "next-intl/server"
import { DocsShell } from "@/components/docs/docs-shell"
import { CodeBlock } from "@/components/docs/code-block"

export default async function DocsCLIPage() {
  const t = await getTranslations("docsCliPage")

  return (
    <DocsShell
      current="cli"
      title={t("title")}
      description={t("description")}
      toc={[
        { id: "positioning", label: t("toc.positioning") },
        { id: "global-context", label: t("toc.globalContext") },
        { id: "command-groups", label: t("toc.commandGroups") },
        { id: "task-recipes", label: t("toc.taskRecipes") },
        { id: "pull-local-test", label: t("toc.pullLocalTest") },
        { id: "troubleshooting", label: t("toc.troubleshooting") },
      ]}
    >
      <section id="positioning" className="scroll-mt-24">
        <h2 className="text-3xl font-semibold tracking-tight">{t("sections.positioning.title")}</h2>
        <ul className="mt-4 list-disc space-y-3 pl-6 text-lg leading-8">
          <li>
            <strong>{t("sections.positioning.items.novaBinary.label")}:</strong>{" "}
            {t("sections.positioning.items.novaBinary.value")} <code>nova daemon</code>.
          </li>
          <li>
            <strong>{t("sections.positioning.items.orbitCli.label")}:</strong>{" "}
            {t("sections.positioning.items.orbitCli.value")}
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
        <h2 className="text-3xl font-semibold tracking-tight">{t("sections.globalContext.title")}</h2>
        <p className="mt-4 text-lg leading-8 text-muted-foreground">
          {t("sections.globalContext.description")}
        </p>
        <CodeBlock
          code={`# Flags
--server      Zenith gateway URL (Nova-compatible API endpoint)
--api-key     API key
--tenant      Tenant ID
--namespace   Namespace
--output      table|wide|json|yaml

# Environment variables
ZENITH_URL
NOVA_API_KEY
NOVA_TENANT
NOVA_NAMESPACE
NOVA_OUTPUT

# Persist defaults
# 9000 is the unified Zenith entrypoint
orbit config set server http://localhost:9000
orbit config set tenant default
orbit config set namespace default
orbit config set output table`}
        />
      </section>

      <section id="command-groups" className="scroll-mt-24">
        <h2 className="text-3xl font-semibold tracking-tight">{t("sections.commandGroups.title")}</h2>
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
        <h2 className="text-3xl font-semibold tracking-tight">{t("sections.taskRecipes.title")}</h2>
        <p className="mt-4 text-lg leading-8 text-muted-foreground">
          {t("sections.taskRecipes.description")}
        </p>
        <CodeBlock
          code={`# 1) Function lifecycle
orbit functions create \
  --name echo \
  --runtime python \
  --handler main.handler \
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
        <h2 className="text-3xl font-semibold tracking-tight">{t("sections.pullLocalTest.title")}</h2>
        <p className="mt-4 text-lg leading-8 text-muted-foreground">
          {t("sections.pullLocalTest.description")}
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
          <li>{t("sections.pullLocalTest.items.item1")}</li>
          <li>{t("sections.pullLocalTest.items.item2")}</li>
          <li>
            {t("sections.pullLocalTest.items.item3Prefix")} <code>payload.json</code> {t("sections.pullLocalTest.items.item3Middle")} <code>function.meta.json</code>.
          </li>
        </ul>
      </section>

      <section id="troubleshooting" className="scroll-mt-24">
        <h2 className="text-3xl font-semibold tracking-tight">{t("sections.troubleshooting.title")}</h2>
        <ul className="mt-4 list-disc space-y-3 pl-6 text-lg leading-8">
          <li>{t("sections.troubleshooting.items.item1Prefix")} <code>--tenant</code> {t("sections.troubleshooting.items.item1Middle")} <code>--namespace</code> {t("sections.troubleshooting.items.item1Suffix")}</li>
          <li>{t("sections.troubleshooting.items.item2Prefix")} <code>--api-key</code> {t("sections.troubleshooting.items.item2Suffix")}</li>
          <li>{t("sections.troubleshooting.items.item3")}</li>
          <li>{t("sections.troubleshooting.items.item4Prefix")} <code>--output json</code> {t("sections.troubleshooting.items.item4Suffix")}</li>
        </ul>
      </section>
    </DocsShell>
  )
}
