import { DocsShell } from "@/components/docs/docs-shell"
import { CodeBlock } from "@/components/docs/code-block"

export default function DocsInstallationPage() {
  return (
    <DocsShell
      current="installation"
      title="Installation"
      description="Linux x86_64 zero-to-service installation guide based on scripts/setup.sh. Includes full build flow, deployment modes, and component responsibilities."
      toc={[
        { id: "before-you-begin", label: "Before You Begin" },
        { id: "deployment-modes", label: "Deployment Modes" },
        { id: "dev-mode-docker", label: "Mode A: Docker Dev" },
        { id: "linux-zero-to-service", label: "Mode B: Linux Zero-to-Service" },
        { id: "what-setup-does", label: "What setup.sh Does" },
        { id: "component-roles", label: "Component Roles" },
        { id: "verify-installation", label: "Verify Installation" },
        { id: "orbit-and-atlas", label: "Orbit and Atlas" },
        { id: "operations-checklist", label: "Operations Checklist" },
      ]}
    >
      <section id="before-you-begin" className="scroll-mt-24">
        <h2 className="text-3xl font-semibold tracking-tight">Before You Begin</h2>
        <p className="mt-4 text-lg leading-8 text-muted-foreground">
          The one-click script targets Linux x86_64 servers and expects prebuilt Nova binaries.
        </p>
        <ul className="mt-4 list-disc space-y-3 pl-6 text-lg leading-8">
          <li>OS/Arch: Linux + x86_64</li>
          <li>Privileges: root (run with <code>sudo</code>)</li>
          <li>Hardware: <code>/dev/kvm</code> recommended for Firecracker VM execution</li>
          <li>Build toolchain: Go <code>1.24+</code>, make, git, curl, unzip, e2fsprogs</li>
          <li>Ports: <code>9000</code> (Zenith gateway/API entrypoint), <code>3000</code> (Lumen UI), <code>5432</code> (PostgreSQL)</li>
        </ul>
        <div className="mt-4 rounded-lg border border-amber-500/40 bg-amber-500/10 p-4 text-sm text-amber-700 dark:text-amber-400">
          <p className="font-medium">Data safety note</p>
          <p className="mt-1">
            <code>scripts/setup.sh</code> performs a fresh deployment and recreates the <code>nova</code> database.
            Existing data will be removed.
          </p>
        </div>
      </section>

      <section id="deployment-modes" className="scroll-mt-24">
        <h2 className="text-3xl font-semibold tracking-tight">Deployment Modes</h2>
        <p className="mt-4 text-lg leading-8 text-muted-foreground">
          Nova supports two practical installation modes:
        </p>
        <ul className="mt-4 list-disc space-y-3 pl-6 text-lg leading-8">
          <li>
            <strong>Mode A (Docker Dev):</strong> fastest local development path, containerized stack.
          </li>
          <li>
            <strong>Mode B (Linux Zero-to-Service):</strong> build binaries + run <code>scripts/setup.sh</code> for
            host-level deployment with systemd.
          </li>
        </ul>
      </section>

      <section id="dev-mode-docker" className="scroll-mt-24">
        <h2 className="text-3xl font-semibold tracking-tight">Mode A: Docker Dev</h2>
        <p className="mt-4 text-lg leading-8 text-muted-foreground">
          Use this when you need a quick local environment for feature work and docs browsing.
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
        <h2 className="text-3xl font-semibold tracking-tight">Mode B: Linux Zero-to-Service</h2>
        <p className="mt-4 text-lg leading-8 text-muted-foreground">
          This is the full from-scratch flow on Linux host. It builds binaries and lets <code>setup.sh</code>
          install PostgreSQL, Firecracker, rootfs images, Nova, and Lumen as systemd services.
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
          Remote deployment variant (build locally, run setup remotely):
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
        <h2 className="text-3xl font-semibold tracking-tight">What setup.sh Does</h2>
        <p className="mt-4 text-lg leading-8 text-muted-foreground">
          Internally, <code>scripts/setup.sh</code> executes these stages in order:
        </p>
        <ol className="mt-4 list-decimal space-y-3 pl-6 text-lg leading-8">
          <li>Checks root permissions, Linux/x86_64, and binary artifacts.</li>
          <li>Installs host dependencies and Node.js 20.</li>
          <li>Resets previous installation state under <code>/opt/nova</code> and <code>/tmp/nova</code>.</li>
          <li>Installs and initializes PostgreSQL; recreates <code>nova</code> role/database.</li>
          <li>Applies <code>scripts/init-db.sql</code> schema and default records.</li>
          <li>Installs Firecracker + Jailer and downloads Firecracker-compatible kernel.</li>
          <li>Deploys Nova binaries and generates <code>/opt/nova/configs/nova.json</code>.</li>
          <li>Builds all runtime rootfs images (<code>.ext4</code>), each embedding <code>nova-agent</code> as <code>/init</code>.</li>
          <li>Builds Lumen (Next.js standalone) and deploys to <code>/opt/nova/lumen</code>.</li>
          <li>Creates and starts <code>nova.service</code> and <code>lumen.service</code>.</li>
          <li>Seeds sample functions via Nova API for initial validation.</li>
        </ol>
      </section>

      <section id="component-roles" className="scroll-mt-24">
        <h2 className="text-3xl font-semibold tracking-tight">Component Roles</h2>
        <p className="mt-4 text-lg leading-8 text-muted-foreground">
          Components used in the full Linux deployment and what each one does:
        </p>
        <div className="mt-5 overflow-x-auto rounded-lg border border-border">
          <table className="w-full min-w-[980px] text-sm">
            <thead>
              <tr className="border-b border-border bg-muted/30">
                <th className="px-3 py-2 text-left font-medium text-muted-foreground">Component</th>
                <th className="px-3 py-2 text-left font-medium text-muted-foreground">Type</th>
                <th className="px-3 py-2 text-left font-medium text-muted-foreground">Role</th>
              </tr>
            </thead>
            <tbody>
              <tr className="border-b border-border">
                <td className="px-3 py-2 font-mono text-xs">postgresql</td>
                <td className="px-3 py-2 text-muted-foreground">Database</td>
                <td className="px-3 py-2 text-muted-foreground">Persists control-plane state, metadata, logs, keys, and configuration.</td>
              </tr>
              <tr className="border-b border-border">
                <td className="px-3 py-2 font-mono text-xs">scripts/init-db.sql</td>
                <td className="px-3 py-2 text-muted-foreground">Schema bootstrap</td>
                <td className="px-3 py-2 text-muted-foreground">Creates tables/indexes and default records required by Nova.</td>
              </tr>
              <tr className="border-b border-border">
                <td className="px-3 py-2 font-mono text-xs">/opt/nova/bin/nova</td>
                <td className="px-3 py-2 text-muted-foreground">Backend service</td>
                <td className="px-3 py-2 text-muted-foreground">Runs Nova API/control plane on port 9000.</td>
              </tr>
              <tr className="border-b border-border">
                <td className="px-3 py-2 font-mono text-xs">/opt/nova/bin/nova-agent</td>
                <td className="px-3 py-2 text-muted-foreground">Guest runtime agent</td>
                <td className="px-3 py-2 text-muted-foreground">Executed inside VM rootfs as /init to run function workloads.</td>
              </tr>
              <tr className="border-b border-border">
                <td className="px-3 py-2 font-mono text-xs">firecracker + jailer</td>
                <td className="px-3 py-2 text-muted-foreground">VM runtime</td>
                <td className="px-3 py-2 text-muted-foreground">Provides lightweight microVM isolation for function execution.</td>
              </tr>
              <tr className="border-b border-border">
                <td className="px-3 py-2 font-mono text-xs">/opt/nova/kernel/vmlinux</td>
                <td className="px-3 py-2 text-muted-foreground">Kernel artifact</td>
                <td className="px-3 py-2 text-muted-foreground">Linux kernel used by Firecracker guest VMs.</td>
              </tr>
              <tr className="border-b border-border">
                <td className="px-3 py-2 font-mono text-xs">/opt/nova/rootfs/*.ext4</td>
                <td className="px-3 py-2 text-muted-foreground">Runtime images</td>
                <td className="px-3 py-2 text-muted-foreground">Language-specific root filesystems (python/node/go/java/...)</td>
              </tr>
              <tr className="border-b border-border">
                <td className="px-3 py-2 font-mono text-xs">lumen.service</td>
                <td className="px-3 py-2 text-muted-foreground">UI service</td>
                <td className="px-3 py-2 text-muted-foreground">Runs Lumen dashboard (Next.js standalone) on port 3000.</td>
              </tr>
              <tr className="border-b border-border">
                <td className="px-3 py-2 font-mono text-xs">nova.service</td>
                <td className="px-3 py-2 text-muted-foreground">systemd unit</td>
                <td className="px-3 py-2 text-muted-foreground">Supervises Nova daemon process and restart policy.</td>
              </tr>
              <tr className="border-b border-border">
                <td className="px-3 py-2 font-mono text-xs">/opt/nova</td>
                <td className="px-3 py-2 text-muted-foreground">Install root</td>
                <td className="px-3 py-2 text-muted-foreground">Holds binaries, configs, kernel, rootfs, snapshots, and UI bundle.</td>
              </tr>
              <tr>
                <td className="px-3 py-2 font-mono text-xs">/tmp/nova</td>
                <td className="px-3 py-2 text-muted-foreground">Runtime temp dir</td>
                <td className="px-3 py-2 text-muted-foreground">Stores sockets, vsock files, and execution logs during runtime.</td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>

      <section id="verify-installation" className="scroll-mt-24">
        <h2 className="text-3xl font-semibold tracking-tight">Verify Installation</h2>
        <ol className="mt-4 list-decimal space-y-3 pl-6 text-lg leading-8">
          <li>Check systemd status for <code>postgresql</code>, <code>nova</code>, and <code>lumen</code>.</li>
          <li>Check backend health endpoints (Zenith entrypoint).</li>
          <li>Call API once to confirm function lifecycle works.</li>
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
        <h2 className="text-3xl font-semibold tracking-tight">Orbit and Atlas</h2>
        <p className="mt-4 text-lg leading-8 text-muted-foreground">
          <code>setup.sh</code> deploys backend and UI. Orbit CLI and Atlas MCP are built separately when needed.
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
        <h2 className="text-3xl font-semibold tracking-tight">Operations Checklist</h2>
        <ul className="mt-4 list-disc space-y-3 pl-6 text-lg leading-8">
          <li>Enable auth and tenant governance before exposing APIs outside trusted networks.</li>
          <li>Back up PostgreSQL and verify restore procedures regularly.</li>
          <li>Pin Firecracker/kernel versions in controlled environments instead of floating latest.</li>
          <li>Monitor <code>journalctl -u nova</code> and <code>journalctl -u lumen</code> for runtime issues.</li>
          <li>Set up metrics/tracing exporters and retention policy for invocation logs.</li>
          <li>Treat rerunning <code>setup.sh</code> as a re-provision operation, not an in-place upgrade.</li>
        </ul>
      </section>

      <section id="production-checklist" className="scroll-mt-24">
        <h2 className="text-3xl font-semibold tracking-tight">Daily Commands</h2>
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
