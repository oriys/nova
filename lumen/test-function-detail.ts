import { chromium } from '@playwright/test';

async function testFunctionDetail() {
  const browser = await chromium.launch();
  const page = await browser.newPage();

  page.on('console', msg => {
    console.log(`[${msg.type()}] ${msg.text()}`);
  });

  page.on('pageerror', err => {
    console.log(`[JS Error] ${err.message}`);
  });

  page.on('response', response => {
    if (response.status() >= 400) {
      console.log(`[HTTP ${response.status()}] ${response.url()}`);
    }
  });

  try {
    console.log('Navigating to function detail...');
    await page.goto('http://localhost:3000/functions/hello-python', {
      waitUntil: 'domcontentloaded',
      timeout: 30000
    });

    await page.waitForTimeout(5000);
    await page.screenshot({ path: 'test-results/function-detail.png', fullPage: true });
    console.log('Screenshot saved');

  } catch (err: any) {
    console.error('Test failed:', err.message);
  } finally {
    await browser.close();
  }
}

testFunctionDetail();
