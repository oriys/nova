import { expect, test, type Page } from "@playwright/test"

const paged = <T>(items: T[]) => ({
  items,
  pagination: { total: items.length },
})

async function mockDashboardApis(page: Page) {
  await page.route("**/api/**", async (route) => {
    const url = new URL(route.request().url())
    const path = url.pathname.replace(/^\/api/, "")

    if (path === "/functions") {
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(
          paged([
            {
              id: "fn-1",
              name: "hello-world",
              runtime: "python3.11",
              handler: "handler",
              memory_mb: 128,
              timeout_s: 10,
              min_replicas: 0,
              created_at: "2026-01-01T00:00:00Z",
              updated_at: "2026-01-01T00:00:00Z",
            },
          ]),
        ),
      })
    }

    if (path === "/metrics") {
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          uptime_seconds: 3600,
          invocations: {
            total: 10,
            success: 9,
            failed: 1,
            cold: 2,
            warm: 8,
            cold_pct: 20,
          },
          latency_ms: { avg: 100, min: 40, max: 220 },
          vms: { created: 2, stopped: 1, crashed: 0, snapshots_hit: 0 },
          functions: {},
        }),
      })
    }

    if (path === "/metrics/timeseries") {
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify([
          {
            timestamp: "2026-01-01T00:00:00Z",
            invocations: 10,
            errors: 1,
            avg_duration: 100,
          },
        ]),
      })
    }

    if (path === "/gateway/routes") {
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(paged([])),
      })
    }

    if (path === "/health") {
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          status: "healthy",
          components: { nova: "healthy", comet: "healthy", zenith: "healthy" },
          uptime_seconds: 3600,
        }),
      })
    }

    if (path === "/notifications/unread-count") {
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ unread: 0 }),
      })
    }

    if (path === "/notifications") {
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(paged([])),
      })
    }

    if (path === "/tenants/default/menu-permissions") {
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(
          paged([
            { menu_key: "dashboard", enabled: true },
            { menu_key: "functions", enabled: true },
          ]),
        ),
      })
    }

    if (path === "/functions/hello-world/logs") {
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify([]),
      })
    }

    return route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({}),
    })
  })
}

test.describe("Lumen e2e auth flow", () => {
  test("redirects guests from dashboard to login", async ({ page }) => {
    await page.goto("/dashboard")
    await expect(page).toHaveURL(/\/login\?next=%2Fdashboard/)
    await expect(page.getByRole("heading", { name: "User Login" })).toBeVisible()
  })

  test("shows inline error on invalid credentials", async ({ page }) => {
    await page.goto("/login")
    await page.getByLabel("Username").fill("admin")
    await page.getByLabel("Password").fill("wrong")
    await page.getByRole("button", { name: "Sign in" }).click()
    await expect(page.getByText("Invalid username or password.")).toBeVisible()
  })

  test("allows login and loads dashboard with mocked backend", async ({ page }) => {
    await mockDashboardApis(page)
    await page.goto("/login")
    await page.getByRole("button", { name: "Sign in" }).click()
    await expect(page).toHaveURL("/dashboard")
    await expect(page.getByRole("heading", { name: "Dashboard" })).toBeVisible()
    await expect(page.getByText("Total Invocations")).toBeVisible()
  })
})
