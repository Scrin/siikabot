import { existsSync } from 'node:fs'
import { chromium, defineConfig } from '@playwright/test'

// The dashboard in a browser. The Go end-to-end test TestDashboardInABrowser runs these tests against a bot it has
// set up, and tells them about it in SIIKABOT_E2E_WORLD. Run them through it:
//   go test -tags e2e ./e2e -run TestDashboardInABrowser

// The browser: SIIKABOT_E2E_CHROMIUM if it's set, Playwright's own Chromium if `npx playwright install chromium`
// has put it in place, as in CI, and the system's Chromium otherwise
function chromiumPath(): string | undefined {
  if (process.env.SIIKABOT_E2E_CHROMIUM) return process.env.SIIKABOT_E2E_CHROMIUM
  if (existsSync(chromium.executablePath())) return undefined
  return ['/usr/bin/chromium', '/usr/bin/chromium-browser', '/usr/bin/google-chrome'].find((path) =>
    existsSync(path),
  )
}

export default defineConfig({
  testDir: './e2e',
  // The tests share one bot, and each person has one session at a time, so they run one after another
  workers: 1,
  fullyParallel: false,
  forbidOnly: true,
  retries: 0,
  timeout: 60_000,
  // Logging in waits for the bot to handle !auth and for the dashboard's next poll
  expect: { timeout: 15_000 },
  reporter: 'list',
  outputDir: process.env.SIIKABOT_E2E_BROWSER_OUTPUT ?? 'test-results',
  use: {
    browserName: 'chromium',
    launchOptions: { executablePath: chromiumPath() },
    reducedMotion: 'reduce',
    trace: 'retain-on-failure',
  },
})
