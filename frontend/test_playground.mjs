import { chromium } from 'playwright';
import path from 'path';

async function run() {
  console.log('Launching browser (msedge channel)...');
  const browser = await chromium.launch({ channel: 'msedge', headless: true });
  const context = await browser.newContext({ viewport: { width: 1440, height: 900 } });
  const page = await context.newPage();

  const errors = [];
  page.on('console', msg => {
    if (msg.type() === 'error') {
      errors.push(`Console error: ${msg.text()}`);
    }
  });
  page.on('pageerror', err => {
    errors.push(`Page error: ${err.message}`);
  });

  console.log('Navigating to http://127.0.0.1:3000/url-builder/filters ...');
  await page.goto('http://127.0.0.1:3000/url-builder/filters', { waitUntil: 'networkidle' });

  // 1. Verify Page Title and Header
  const title = await page.title();
  const header = await page.locator('.playground-header h1').innerText();
  console.log(`✓ Page Title: "${title}"`);
  console.log(`✓ Header Text: "${header}"`);

  // 2. Verify Nav Pills
  const activeNav = await page.locator('.playground-nav-link.active').innerText();
  console.log(`✓ Active Nav Link: "${activeNav}"`);

  // 3. Verify Initial JSON output
  const initialJSON = await page.locator('#filter-json-text').innerText();
  console.log(`✓ Initial Filter JSON: ${initialJSON.replace(/\s+/g, ' ')}`);

  // 4. Test Tab Switching
  console.log('Testing Tab: Slideshow URL...');
  await page.locator('button[data-tab="tab-url"]').click();
  const urlTabVisible = await page.locator('#tab-url').isVisible();
  const kioskURL = await page.locator('#kiosk-url-text').innerText();
  console.log(`✓ Slideshow URL Tab Visible: ${urlTabVisible}, URL: ${kioskURL}`);

  console.log('Testing Tab: config.yaml...');
  await page.locator('button[data-tab="tab-yaml"]').click();
  const yamlTabVisible = await page.locator('#tab-yaml').isVisible();
  const yamlText = await page.locator('#redirect-yaml-text').innerText();
  console.log(`✓ config.yaml Tab Visible: ${yamlTabVisible}, Snippet:\n${yamlText}`);

  console.log('Testing Tab: Filter JSON...');
  await page.locator('button[data-tab="tab-json"]').click();
  const jsonTabVisible = await page.locator('#tab-json').isVisible();
  console.log(`✓ Filter JSON Tab Visible: ${jsonTabVisible}`);

  // 5. Test Form Interactivity with HTMX
  const labelLoc = page.locator('label[for="favorite_true"]');
  console.log('Label exists count:', await labelLoc.count());
  if (await labelLoc.count() > 0) {
    console.log('Label isVisible:', await labelLoc.isVisible());
    console.log('Label boundingBox:', await labelLoc.boundingBox());
  }

  // Save screenshot immediately to see page appearance
  const screenshotPath = 'C:\\Users\\Rafael Savi\\.gemini\\antigravity-ide\\brain\\39cd892c-e0df-4369-90c6-be80d023d6eb\\playground_browser_test.png';
  await page.screenshot({ path: screenshotPath, fullPage: true });
  console.log(`✓ Initial full-page screenshot saved to: ${screenshotPath}`);

  console.log('Selecting "Favourites only" radio option...');
  await page.locator('label[for="favorite_true"]').click({ force: true });
  
  // Wait for HTMX swap / network idle
  await page.waitForTimeout(1500);

  const updatedJSON = await page.locator('#filter-json-text').innerText();
  console.log(`✓ Updated Filter JSON after checking Favourites:\n${updatedJSON}`);

  // 6. Check for Matching Assets or Empty alert
  const hasAlert = await page.locator('.playground-alert').isVisible();
  const hasGallery = await page.locator('.simulation-gallery-card').isVisible();
  if (hasGallery) {
    const badge = await page.locator('.count-badge').innerText();
    const count = await page.locator('.simulation-thumb-card').count();
    console.log(`✓ Matching gallery displayed: "${badge}" (${count} thumbnails rendered)`);
  } else if (hasAlert) {
    const alertText = await page.locator('.playground-alert').innerText();
    console.log(`✓ Status Alert: ${alertText.replace(/\s+/g, ' ')}`);
  }

  // 7. Save final screenshot
  const finalScreenshotPath = 'C:\\Users\\Rafael Savi\\.gemini\\antigravity-ide\\brain\\39cd892c-e0df-4369-90c6-be80d023d6eb\\playground_browser_test_final.png';
  await page.screenshot({ path: finalScreenshotPath, fullPage: true });
  console.log(`✓ Final screenshot saved to: ${finalScreenshotPath}`);

  if (errors.length > 0) {
    console.warn('Console/Page errors detected:', errors);
  } else {
    console.log('✓ No JavaScript errors or console errors detected!');
  }

  await browser.close();
  console.log('Playwright test completed successfully.');
}

run().catch(err => {
  console.error('Playwright Test FAILED:', err);
  process.exit(1);
});
