import { defineConfig, devices } from '@playwright/test'

export default defineConfig({
  testDir: './e2e',
  fullyParallel: true,
  forbidOnly: Boolean(process.env.CI),
  retries: 0,
  use: { baseURL: 'http://127.0.0.1:5182', trace: 'retain-on-failure', locale: 'zh-CN' },
  reporter: process.env.CI ? 'github' : 'list',
  projects: [
    {
      name: 'desktop',
      testIgnore: '**/production.spec.ts',
      use: { ...devices['Desktop Chrome'], viewport: { width: 1440, height: 1000 } },
    },
    {
      name: 'mobile',
      testIgnore: '**/production.spec.ts',
      use: { ...devices['Pixel 7'], defaultBrowserType: 'chromium' },
    },
    {
      name: 'production',
      testMatch: '**/production.spec.ts',
      use: { ...devices['Desktop Chrome'], baseURL: 'http://127.0.0.1:5183' },
    },
  ],
  webServer: [
    {
      command: 'pnpm exec vite --host 127.0.0.1 --port 5182 --strictPort',
      url: 'http://127.0.0.1:5182',
      reuseExistingServer: false,
    },
    {
      command: 'pnpm build && pnpm exec vite preview --host 127.0.0.1 --port 5183 --strictPort',
      url: 'http://127.0.0.1:5183',
      reuseExistingServer: false,
    },
  ],
})
