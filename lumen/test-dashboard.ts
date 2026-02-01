import { chromium } from '@playwright/test';

async function testDashboard() {
  const browser = await chromium.launch();
  const page = await browser.newPage();

  console.log('Navigating to dashboard...');

  page.on('console', msg => {
    if (msg.type() === 'error') {
      console.log('Console error:', msg.text());
    }
  });

  page.on('pageerror', err => {
    console.log('Page error:', err.message);
  });

  try {
    await page.goto('http://nova-lumen.orb.local:3000', { waitUntil: 'networkidle' });

    console.log('Page title:', await page.title());

    // Wait for dashboard content
    await page.waitForTimeout(3000);

    // Take screenshot
    await page.screenshot({ path: 'test-results/dashboard.png', fullPage: true });
    console.log('Screenshot saved to test-results/dashboard.png');

    // Check for error messages
    const errorText = await page.locator('text=Failed').count();
    if (errorText > 0) {
      console.log('Found "Failed" text on page');
      const content = await page.content();
      console.log('Page contains error indicators');
    } else {
      console.log('No errors found on page');
    }

    // Get stats cards content
    const statsCards = await page.locator('.text-3xl').allTextContents();
    console.log('Stats values:', statsCards);

  } catch (err) {
    console.error('Test failed:', err);
  } finally {
    await browser.close();
  }
}

testDashboard();
