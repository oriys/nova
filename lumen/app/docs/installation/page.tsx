import { getTranslations } from "next-intl/server"
import { DocsShell } from "@/components/docs/docs-shell"
import { CodeBlock } from "@/components/docs/code-block"

export default async function DocsInstallationPage() {
  const t = await getTranslations("docsInstallationPage")

  return (
    <DocsShell
      current="installation"
      title={t("title")}
      description={t("description")}
      toc={[
        { id: "before-you-begin", label: t("toc.beforeYouBegin") },
        { id: "deployment-modes", label: t("toc.deploymentModes") },
        { id: "dev-mode-docker", label: t("toc.devModeDocker") },
        { id: "linux-zero-to-service", label: t("toc.linuxZeroToService") },
        { id: "what-setup-does", label: t("toc.whatSetupDoes") },
        { id: "component-roles", label: t("toc.componentRoles") },
        { id: "verify-installation", label: t("toc.verifyInstallation") },
        { id: "orbit-and-atlas", label: t("toc.orbitAndAtlas") },
        { id: "operations-checklist", label: t("toc.operationsChecklist") },
      ]}
    >
      <section id="before-you-begin" className="scroll-mt-24">
        <h2 className="text-3xl font-semibold tracking-tight">{t("sections.beforeYouBegin.title")}</h2>
        <p className="mt-4 text-lg leading-8 text-muted-foreground">
          {t("sections.beforeYouBegin.description")}
        </p>
        <ul className="mt-4 list-disc space-y-3 pl-6 text-lg leading-8">
          <li>{t("sections.beforeYouBegin.items.item1")}</li>
          <li>{t("sections.beforeYouBegin.items.item2Prefix")} <code>sudo</code>{t("sections.beforeYouBegin.items.item2Suffix")}</li>
          <li>{t("sections.beforeYouBegin.items.item3Prefix")} <code>/dev/kvm</code> {t("sections.beforeYouBegin.items.item3Suffix")}</li>
          <li>{t("sections.beforeYouBegin.items.item4Prefix")} <code>1.24+</code>, make, git, curl, unzip, e2fsprogs</li>
          <li>{t("sections.beforeYouBegin.items.item5Prefix")} <code>9000</code> {t("sections.beforeYouBegin.items.item5Middle")} <code>3000</code> {t("sections.beforeYouBegin.items.item5Middle2")} <code>5432</code> {t("sections.beforeYouBegin.items.item5Suffix")}</li>
        </ul>
        <div className="mt-4 rounded-lg border border-amber-500/40 bg-amber-500/10 p-4 text-sm text-amber-700 dark:text-amber-400">
          <p className="font-medium">{t("sections.beforeYouBegin.dataSafetyTitle")}</p>
          <p className="mt-1">
            <code>scripts/setup.sh</code> {t("sections.beforeYouBegin.dataSafetyBodyPrefix")} <code>nova</code> {t("sections.beforeYouBegin.dataSafetyBodySuffix")}
          </p>
        </div>
      </section>

      <section id="deployment-modes" className="scroll-mt-24">
        <h2 className="text-3xl font-semibold tracking-tight">{t("sections.deploymentModes.title")}</h2>
        <p className="mt-4 text-lg leading-8 text-muted-foreground">
          {t("sections.deploymentModes.description")}
        </p>
        <ul className="mt-4 list-disc space-y-3 pl-6 text-lg leading-8">
          <li>
            <strong>{t("sections.deploymentModes.items.modeA.label")}:</strong>{" "}
            {t("sections.deploymentModes.items.modeA.value")}
          </li>
          <li>
            <strong>{t("sections.deploymentModes.items.modeB.label")}:</strong>{" "}
            {t("sections.deploymentModes.items.modeB.valuePrefix")} <code>scripts/setup.sh</code> {t("sections.deploymentModes.items.modeB.valueSuffix")}
          </li>
        </ul>
      </section>

      <section id="dev-mode-docker" className="scroll-mt-24">
        <h2 className="text-3xl font-semibold tracking-tight">{t("sections.devModeDocker.title")}</h2>
        <p className="mt-4 text-lg leading-8 text-muted-foreground">
          {t("sections.devModeDocker.description")}
        </p>
        <CodeBlock
          code={`git clone https://github.com/oriys/nova.git
cd nova

# Start full stack (postgres + nova + comet + zenith + lumen + seeders)
make dev

# Open UI and docs
http://localhost:3000
http://localhost:3000/docs`}
        />
      </section>

      <section id="linux-zero-to-service" className="scroll-mt-24">
        <h2 className="text-3xl font-semibold tracking-tight">{t("sections.linuxZeroToService.title")}</h2>
        <p className="mt-4 text-lg leading-8 text-muted-foreground">
          {t("sections.linuxZeroToService.descriptionPrefix")} <code>setup.sh</code> {t("sections.linuxZeroToService.descriptionSuffix")}
        </p>

        <CodeBlock
          code={`# 0) Host prerequisites (Debian/Ubuntu example)
sudo apt-get update
sudo apt-get install -y git make curl unzip e2fsprogs iproute2

# 1) Clone source
git clone https://github.com/oriys/nova.git
cd nova

# 2) Ensure Go 1.24+ is installed
go version

# 3) Build Linux artifacts required by setup.sh
make build-linux
# expected:
#   bin/nova-linux
#   bin/nova-agent

# 4) One-click deployment (root required)
sudo bash scripts/setup.sh

# 5) Verify services
systemctl status postgresql nova lumen --no-pager
curl -sf http://127.0.0.1:9000/health
curl -I http://127.0.0.1:3000`}
        />

        <p className="mt-4 text-lg leading-8 text-muted-foreground">
          {t("sections.linuxZeroToService.remoteVariant")}
        </p>
        <CodeBlock
          code={`# Local machine
make build-linux
scp -r scripts/ bin/ lumen/ user@server:/tmp/nova-deploy/

# Remote machine
ssh user@server 'sudo bash /tmp/nova-deploy/scripts/setup.sh'`}
        />
      </section>

      <section id="what-setup-does" className="scroll-mt-24">
        <h2 className="text-3xl font-semibold tracking-tight">{t("sections.whatSetupDoes.title")}</h2>
        <p className="mt-4 text-lg leading-8 text-muted-foreground">
          {t("sections.whatSetupDoes.descriptionPrefix")} <code>scripts/setup.sh</code> {t("sections.whatSetupDoes.descriptionSuffix")}
        </p>
        <ol className="mt-4 list-decimal space-y-3 pl-6 text-lg leading-8">
          <li>{t("sections.whatSetupDoes.items.item1")}</li>
          <li>{t("sections.whatSetupDoes.items.item2")}</li>
          <li>{t("sections.whatSetupDoes.items.item3Prefix")} <code>/opt/nova</code> {t("sections.whatSetupDoes.items.item3Middle")} <code>/tmp/nova</code>.</li>
          <li>{t("sections.whatSetupDoes.items.item4Prefix")} <code>nova</code> {t("sections.whatSetupDoes.items.item4Suffix")}</li>
          <li>{t("sections.whatSetupDoes.items.item5Prefix")} <code>scripts/init-db.sql</code> {t("sections.whatSetupDoes.items.item5Suffix")}</li>
          <li>{t("sections.whatSetupDoes.items.item6")}</li>
          <li>{t("sections.whatSetupDoes.items.item7Prefix")} <code>/opt/nova/configs/nova.json</code>.</li>
          <li>{t("sections.whatSetupDoes.items.item8Prefix")} <code>.ext4</code>{t("sections.whatSetupDoes.items.item8Middle")} <code>nova-agent</code> {t("sections.whatSetupDoes.items.item8Suffix")} <code>/init</code>.</li>
          <li>{t("sections.whatSetupDoes.items.item9Prefix")} <code>/opt/nova/lumen</code>.</li>
          <li>{t("sections.whatSetupDoes.items.item10Prefix")} <code>nova.service</code> {t("sections.whatSetupDoes.items.item10Middle")} <code>lumen.service</code>.</li>
          <li>{t("sections.whatSetupDoes.items.item11")}</li>
        </ol>
      </section>

      <section id="component-roles" className="scroll-mt-24">
        <h2 className="text-3xl font-semibold tracking-tight">{t("sections.componentRoles.title")}</h2>
        <p className="mt-4 text-lg leading-8 text-muted-foreground">
          {t("sections.componentRoles.description")}
        </p>
        <div className="mt-5 overflow-x-auto rounded-lg border border-border">
          <table className="w-full min-w-[980px] text-sm">
            <thead>
              <tr className="border-b border-border bg-muted/30">
                <th className="px-3 py-2 text-left font-medium text-muted-foreground">{t("sections.componentRoles.table.component")}</th>
                <th className="px-3 py-2 text-left font-medium text-muted-foreground">{t("sections.componentRoles.table.type")}</th>
                <th className="px-3 py-2 text-left font-medium text-muted-foreground">{t("sections.componentRoles.table.role")}</th>
              </tr>
            </thead>
            <tbody>
              <tr className="border-b border-border">
                <td className="px-3 py-2 font-mono text-xs">postgresql</td>
                <td className="px-3 py-2 text-muted-foreground">{t("sections.componentRoles.rows.postgresql.type")}</td>
                <td className="px-3 py-2 text-muted-foreground">{t("sections.componentRoles.rows.postgresql.role")}</td>
              </tr>
              <tr className="border-b border-border">
                <td className="px-3 py-2 font-mono text-xs">scripts/init-db.sql</td>
                <td className="px-3 py-2 text-muted-foreground">{t("sections.componentRoles.rows.initDb.type")}</td>
                <td className="px-3 py-2 text-muted-foreground">{t("sections.componentRoles.rows.initDb.role")}</td>
              </tr>
              <tr className="border-b border-border">
                <td className="px-3 py-2 font-mono text-xs">/opt/nova/bin/nova</td>
                <td className="px-3 py-2 text-muted-foreground">{t("sections.componentRoles.rows.novaBinary.type")}</td>
                <td className="px-3 py-2 text-muted-foreground">{t("sections.componentRoles.rows.novaBinary.role")}</td>
              </tr>
              <tr className="border-b border-border">
                <td className="px-3 py-2 font-mono text-xs">/opt/nova/bin/nova-agent</td>
                <td className="px-3 py-2 text-muted-foreground">{t("sections.componentRoles.rows.novaAgent.type")}</td>
                <td className="px-3 py-2 text-muted-foreground">{t("sections.componentRoles.rows.novaAgent.role")}</td>
              </tr>
              <tr className="border-b border-border">
                <td className="px-3 py-2 font-mono text-xs">firecracker + jailer</td>
                <td className="px-3 py-2 text-muted-foreground">{t("sections.componentRoles.rows.firecracker.type")}</td>
                <td className="px-3 py-2 text-muted-foreground">{t("sections.componentRoles.rows.firecracker.role")}</td>
              </tr>
              <tr className="border-b border-border">
                <td className="px-3 py-2 font-mono text-xs">/opt/nova/kernel/vmlinux</td>
                <td className="px-3 py-2 text-muted-foreground">{t("sections.componentRoles.rows.kernel.type")}</td>
                <td className="px-3 py-2 text-muted-foreground">{t("sections.componentRoles.rows.kernel.role")}</td>
              </tr>
              <tr className="border-b border-border">
                <td className="px-3 py-2 font-mono text-xs">/opt/nova/rootfs/*.ext4</td>
                <td className="px-3 py-2 text-muted-foreground">{t("sections.componentRoles.rows.rootfs.type")}</td>
                <td className="px-3 py-2 text-muted-foreground">{t("sections.componentRoles.rows.rootfs.role")}</td>
              </tr>
              <tr className="border-b border-border">
                <td className="px-3 py-2 font-mono text-xs">lumen.service</td>
                <td className="px-3 py-2 text-muted-foreground">{t("sections.componentRoles.rows.lumenService.type")}</td>
                <td className="px-3 py-2 text-muted-foreground">{t("sections.componentRoles.rows.lumenService.role")}</td>
              </tr>
              <tr className="border-b border-border">
                <td className="px-3 py-2 font-mono text-xs">nova.service</td>
                <td className="px-3 py-2 text-muted-foreground">{t("sections.componentRoles.rows.novaService.type")}</td>
                <td className="px-3 py-2 text-muted-foreground">{t("sections.componentRoles.rows.novaService.role")}</td>
              </tr>
              <tr className="border-b border-border">
                <td className="px-3 py-2 font-mono text-xs">/opt/nova</td>
                <td className="px-3 py-2 text-muted-foreground">{t("sections.componentRoles.rows.installRoot.type")}</td>
                <td className="px-3 py-2 text-muted-foreground">{t("sections.componentRoles.rows.installRoot.role")}</td>
              </tr>
              <tr>
                <td className="px-3 py-2 font-mono text-xs">/tmp/nova</td>
                <td className="px-3 py-2 text-muted-foreground">{t("sections.componentRoles.rows.runtimeTempDir.type")}</td>
                <td className="px-3 py-2 text-muted-foreground">{t("sections.componentRoles.rows.runtimeTempDir.role")}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>

      <section id="verify-installation" className="scroll-mt-24">
        <h2 className="text-3xl font-semibold tracking-tight">{t("sections.verifyInstallation.title")}</h2>
        <ol className="mt-4 list-decimal space-y-3 pl-6 text-lg leading-8">
          <li>{t("sections.verifyInstallation.items.item1Prefix")} <code>postgresql</code>, <code>nova</code>, {t("sections.verifyInstallation.items.item1Middle")} <code>lumen</code>.</li>
          <li>{t("sections.verifyInstallation.items.item2")}</li>
          <li>{t("sections.verifyInstallation.items.item3")}</li>
        </ol>
        <CodeBlock
          code={`# Services
systemctl status postgresql nova lumen --no-pager

# Health
curl -s http://127.0.0.1:9000/health | jq
curl -s http://127.0.0.1:9000/health/ready | jq

# UI
curl -I http://127.0.0.1:3000

# Optional API smoke test
curl -s -X POST http://127.0.0.1:9000/functions \
  -H "Content-Type: application/json" \
  -d '{"name":"install-check","runtime":"python","handler":"main.handler","code":"def handler(event, context):\\n    return {\\"ok\\": True}"}' | jq`}
        />
      </section>

      <section id="orbit-and-atlas" className="scroll-mt-24">
        <h2 className="text-3xl font-semibold tracking-tight">{t("sections.orbitAndAtlas.title")}</h2>
        <p className="mt-4 text-lg leading-8 text-muted-foreground">
          <code>setup.sh</code> {t("sections.orbitAndAtlas.description")}
        </p>
        <CodeBlock
          code={`# Orbit CLI
make orbit
# 9000 is the unified Zenith entrypoint
./orbit/target/debug/orbit config set server http://127.0.0.1:9000
./orbit/target/debug/orbit config set tenant default
./orbit/target/debug/orbit config set namespace default

# Atlas MCP server
make atlas-linux
# binary: ./bin/atlas-linux
# run with ZENITH_URL/NOVA_API_KEY/NOVA_TENANT/NOVA_NAMESPACE env`}
        />
      </section>

      <section id="operations-checklist" className="scroll-mt-24">
        <h2 className="text-3xl font-semibold tracking-tight">{t("sections.operationsChecklist.title")}</h2>
        <ul className="mt-4 list-disc space-y-3 pl-6 text-lg leading-8">
          <li>{t("sections.operationsChecklist.items.item1")}</li>
          <li>{t("sections.operationsChecklist.items.item2")}</li>
          <li>{t("sections.operationsChecklist.items.item3")}</li>
          <li>{t("sections.operationsChecklist.items.item4Prefix")} <code>journalctl -u nova</code> {t("sections.operationsChecklist.items.item4Middle")} <code>journalctl -u lumen</code> {t("sections.operationsChecklist.items.item4Suffix")}</li>
          <li>{t("sections.operationsChecklist.items.item5")}</li>
          <li>{t("sections.operationsChecklist.items.item6Prefix")} <code>setup.sh</code> {t("sections.operationsChecklist.items.item6Suffix")}</li>
        </ul>
      </section>

      <section id="production-checklist" className="scroll-mt-24">
        <h2 className="text-3xl font-semibold tracking-tight">{t("sections.dailyCommands.title")}</h2>
        <CodeBlock
          code={`# Follow logs
journalctl -u nova -f
journalctl -u lumen -f

# Restart services
sudo systemctl restart nova
sudo systemctl restart lumen

# Check health
curl -s http://127.0.0.1:9000/health | jq`}
        />
      </section>
    </DocsShell>
  )
}
