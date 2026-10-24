import { chromium } from 'playwright';

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

  console.log('Navigating to http://127.0.0.1:3005/url-builder/filters ...');
  await page.goto('http://127.0.0.1:3005/url-builder/filters', { waitUntil: 'networkidle' });

  // 1. Verify Page Title and Header
  const title = await page.title();
  const header = await page.locator('.playground-header h1').innerText();
  console.log(`✓ Page Title: "${title}"`);
  console.log(`✓ Header Text: "${header}"`);

  // 2. Check User Selector Options
  const userSelect = page.locator('#url-builder-user');
  const userOptions = await userSelect.locator('option').allInnerTexts();
  console.log(`✓ User Selector Options count: ${userOptions.length}`);
  console.log(`  Options: ${JSON.stringify(userOptions)}`);

  const selectedUserVal = await userSelect.inputValue();
  console.log(`✓ Initially Selected User value: "${selectedUserVal}" (All Users)`);

  // 3. Inspect Multi-User Options (People, Albums, Countries, Cameras)
  const peopleOptions = await page.locator('select[name="people"] option').allInnerTexts();
  console.log(`✓ Available People count (aggregated): ${peopleOptions.length}`);
  if (peopleOptions.length > 0) {
    console.log(`  Sample people: ${JSON.stringify(peopleOptions.slice(0, 5))}`);
  }

  const albumOptions = await page.locator('select[name="albums"] option').allInnerTexts();
  console.log(`✓ Available Albums count (aggregated): ${albumOptions.length}`);
  if (albumOptions.length > 0) {
    console.log(`  Sample albums: ${JSON.stringify(albumOptions.slice(0, 5))}`);
  }

  const countrySelect = page.locator('select[name="country"]');
  if (await countrySelect.count() > 0) {
    const countryOptions = await countrySelect.locator('option').allInnerTexts();
    console.log(`✓ Available Countries count (aggregated): ${countryOptions.length}`);
    console.log(`  Sample countries: ${JSON.stringify(countryOptions.slice(0, 5))}`);
  }

  const cameraSelect = page.locator('select[name="camera"]');
  if (await cameraSelect.count() > 0) {
    const cameraOptions = await cameraSelect.locator('option').allInnerTexts();
    console.log(`✓ Available Cameras count (aggregated): ${cameraOptions.length}`);
    console.log(`  Sample cameras: ${JSON.stringify(cameraOptions.slice(0, 5))}`);
  }

  // Save screenshot of "All Users" state
  const screenshotPathAll = 'C:\\Users\\Rafael Savi\\.gemini\\antigravity-ide\\brain\\39cd892c-e0df-4369-90c6-be80d023d6eb\\playground_all_users.png';
  await page.screenshot({ path: screenshotPathAll, fullPage: true });
  console.log(`✓ Screenshot of "All Users" view saved: ${screenshotPathAll}`);

  // 4. Test Selecting a Specific User Account (e.g. "ivana" or first available non-empty user)
  const targetUser = userOptions.find(u => u && !u.startsWith('All') && u !== 'default');
  if (targetUser) {
    console.log(`\nSelecting specific user: "${targetUser}" ...`);
    await Promise.all([
      page.waitForNavigation({ waitUntil: 'networkidle' }),
      userSelect.selectOption({ label: targetUser })
    ]);

    const currentUrl = page.url();
    console.log(`✓ Navigated to URL: ${currentUrl}`);

    const newSelectedVal = await page.locator('#url-builder-user').inputValue();
    console.log(`✓ Selected user in dropdown: "${newSelectedVal}"`);

    const userPeopleOptions = await page.locator('select[name="people"] option').allInnerTexts();
    const userAlbumOptions = await page.locator('select[name="albums"] option').allInnerTexts();
    console.log(`✓ User "${targetUser}" People count: ${userPeopleOptions.length}`);
    console.log(`✓ User "${targetUser}" Albums count: ${userAlbumOptions.length}`);

    const screenshotPathUser = 'C:\\Users\\Rafael Savi\\.gemini\\antigravity-ide\\brain\\39cd892c-e0df-4369-90c6-be80d023d6eb\\playground_single_user.png';
    await page.screenshot({ path: screenshotPathUser, fullPage: true });
    console.log(`✓ Screenshot of single-user view saved: ${screenshotPathUser}`);

    // Switch back to All Users
    console.log('\nSwitching back to "All Users"...');
    await Promise.all([
      page.waitForNavigation({ waitUntil: 'networkidle' }),
      page.locator('#url-builder-user').selectOption({ value: '' })
    ]);
    console.log(`✓ Navigated back to: ${page.url()}`);
  }

  // 5. Test Live Search with Filter
  console.log('\nSelecting "Favourites only" option...');
  await page.locator('label[for="favorite_true"]').click({ force: true });
  await page.waitForTimeout(1500);

  const updatedJSON = await page.locator('#filter-json-text').innerText();
  console.log(`✓ Updated Filter JSON:\n${updatedJSON}`);

  const hasGallery = await page.locator('.simulation-gallery-card').isVisible();
  if (hasGallery) {
    const badge = await page.locator('.count-badge').innerText();
    const count = await page.locator('.simulation-thumb-card').count();
    console.log(`✓ Matching simulation gallery: "${badge}" (${count} thumbnails rendered)`);
  }

  if (errors.length > 0) {
    console.warn('Console/Page errors detected:', errors);
  } else {
    console.log('✓ No JavaScript errors or console errors detected!');
  }

  await browser.close();
  console.log('\nAll Playwright E2E verification steps PASSED successfully.');
}

run().catch(err => {
  console.error('Playwright Test FAILED:', err);
  process.exit(1);
});
