// @ts-check
const { test, expect } = require('@playwright/test');

test.describe('Transcriber Pro - Basic Functionality', () => {
  test('homepage loads successfully', async ({ page }) => {
    await page.goto('/');

    // Check for main heading
    await expect(page.locator('h1')).toContainText('Transcriber Pro');

    // Check for upload area
    await expect(page.locator('#dropZone')).toBeVisible();

    // Check for queue section
    await expect(page.locator('#queueSection')).toBeVisible();
  });

  test('connection status is hidden when connected', async ({ page }) => {
    await page.goto('/');

    // Wait for connection
    await page.waitForTimeout(1000);

    // Connection status should be hidden when connected
    const connectionStatus = page.locator('#connectionStatus');
    await expect(connectionStatus).toHaveCSS('display', 'none');
  });

  test('version is displayed in footer', async ({ page }) => {
    await page.goto('/');

    // Version should be visible in footer
    const version = page.locator('#version');
    await expect(version).toBeVisible();
    await expect(version).not.toBeEmpty();
  });

  test('language selector has options', async ({ page }) => {
    await page.goto('/');

    const languageSelect = page.locator('#languageSelect');
    await expect(languageSelect).toBeVisible();

    // Should have auto-detect and multiple language options
    const options = await languageSelect.locator('option').count();
    expect(options).toBeGreaterThan(5);
  });

  test('queue shows empty state initially', async ({ page }) => {
    await page.goto('/');

    // Wait for queue to load
    await page.waitForTimeout(1000);

    // Should show empty placeholder
    const emptyPlaceholder = page.locator('.queue-empty');
    await expect(emptyPlaceholder).toBeVisible();
    await expect(emptyPlaceholder).toContainText('No jobs in queue');
  });

  test('health endpoint responds', async ({ request }) => {
    const response = await request.get('/health');
    expect(response.ok()).toBeTruthy();

    const data = await response.json();
    expect(data.status).toBe('ok');
  });

  test('version endpoint responds', async ({ request }) => {
    const response = await request.get('/version');
    expect(response.ok()).toBeTruthy();

    const data = await response.json();
    expect(data.version).toBeTruthy();
  });

  test('queue endpoint responds', async ({ request }) => {
    const response = await request.get('/queue');
    expect(response.ok()).toBeTruthy();

    const data = await response.json();
    expect(data).toHaveProperty('queue');
    expect(data).toHaveProperty('completed');
    expect(Array.isArray(data.queue)).toBeTruthy();
    expect(Array.isArray(data.completed)).toBeTruthy();
  });
});

test.describe('Transcriber Pro - UI Interactions', () => {
  test('export buttons are hidden initially', async ({ page }) => {
    await page.goto('/');

    // Results section should be hidden
    const resultsSection = page.locator('#resultsSection');
    await expect(resultsSection).toHaveCSS('display', 'none');
  });

  test('two-column layout is displayed', async ({ page }) => {
    await page.goto('/');

    const leftColumn = page.locator('.left-column');
    const rightColumn = page.locator('.right-column');

    await expect(leftColumn).toBeVisible();
    await expect(rightColumn).toBeVisible();
  });

  test('drop zone responds to drag events', async ({ page }) => {
    await page.goto('/');

    const dropZone = page.locator('#dropZone');

    // Simulate drag over
    await dropZone.dispatchEvent('dragover', { bubbles: true, cancelable: true });

    // Should add drag-over class
    await expect(dropZone).toHaveClass(/drag-over/);
  });
});

test.describe('Transcriber Pro - API Interactions', () => {
  test('clear all endpoint responds', async ({ request }) => {
    const response = await request.post('http://localhost:9456/clear-all', {
      timeout: 5000
    });
    expect(response.ok()).toBeTruthy();
  });

  test('clear completed endpoint responds', async ({ request }) => {
    const response = await request.post('http://localhost:9456/clear-completed', {
      timeout: 5000
    });
    expect(response.ok()).toBeTruthy();
  });

  test('transcribe endpoint requires audio file', async ({ request }) => {
    const response = await request.post('http://localhost:9456/transcribe', {
      multipart: {
        language: 'en'
      },
      timeout: 5000
    });

    expect(response.status()).toBe(400);
  });
});
