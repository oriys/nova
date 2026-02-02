import { chromium } from '@playwright/test';

const BASE_URL = 'http://localhost:3000';

const pages = [
  { name: 'Dashboard', path: '/' },
  { name: 'Functions', path: '/functions' },
  { name: 'Runtimes', path: '/runtimes' },
  { name: 'Configurations', path: '/configurations' },
  { name: 'Logs', path: '/logs' },
  { name: 'History', path: '/history' },
];

async function testAllPages() {
  const browser = await chromium.launch();
  const page = await browser.newPage();

  const errors: string[] = [];

  page.on('console', msg => {
    if (msg.type() === 'error') {
      errors.push(`[${page.url()}] Console: ${msg.text()}`);
    }
  });

  page.on('pageerror', err => {
    errors.push(`[${page.url()}] JS Error: ${err.message}`);
  });

  for (const p of pages) {
    console.log(`\n=== Testing ${p.name} (${p.path}) ===`);

    try {
      await page.goto(`${BASE_URL}${p.path}`, { waitUntil: 'networkidle', timeout: 15000 });
      await page.waitForTimeout(2000);

      // Screenshot
      const filename = `test-results/${p.name.toLowerCase()}.png`;
      await page.screenshot({ path: filename, fullPage: true });
      console.log(`Screenshot: ${filename}`);

      // Check for visible error messages
      const failedCount = await page.locator('text=/Failed|Error|Cannot/i').count();
      if (failedCount > 0) {
        console.log(`Warning: Found ${failedCount} potential error messages`);
      } else {
        console.log('OK: No error messages found');
      }

    } catch (err: any) {
      console.log(`FAILED: ${err.message}`);
      errors.push(`[${p.name}] Navigation error: ${err.message}`);
    }
  }

  // Test function detail page
  console.log(`\n=== Testing Function Detail ===`);
  try {
    await page.goto(`${BASE_URL}/functions/hello-python`, { waitUntil: 'networkidle', timeout: 15000 });
    await page.waitForTimeout(2000);
    await page.screenshot({ path: 'test-results/function-detail.png', fullPage: true });
    console.log('Screenshot: test-results/function-detail.png');
    console.log('OK');
  } catch (err: any) {
    console.log(`FAILED: ${err.message}`);
  }

  await browser.close();

  if (errors.length > 0) {
    console.log('\n=== Errors Found ===');
    errors.forEach(e => console.log(e));
  } else {
    console.log('\n=== All pages passed ===');
  }
}

testAllPages();
