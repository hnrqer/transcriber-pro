# Transcriber Pro - E2E Tests

End-to-end tests for Transcriber Pro using Playwright.

## Port Configuration

Tests run on **port 9456** (different from the app's default port 8456) to allow running the app and tests in parallel.

The test server is automatically started by Playwright with environment variables:
- `PORT=9456` - Use test port
- `NO_BROWSER=1` - Disable browser auto-open

## Setup

```bash
cd tests
npm install
npx playwright install
```

## Running Tests

```bash
# Run all tests
npm test

# Run tests with UI
npm run test:ui

# Run tests in headed mode
npm run test:headed

# Debug tests
npm run test:debug
```

## Test Structure

### Basic Functionality Tests (`e2e/basic.spec.js`)

- Homepage loading
- Connection status
- Version display
- Language selector
- Queue empty state
- API health checks

### UI Interaction Tests

- File input handling
- Drag and drop
- Two-column layout
- Button visibility

### API Tests

- Queue endpoints
- Clear operations
- File upload validation

## Writing New Tests

Tests are organized in `e2e/` directory. Each file contains related test suites.

Example test:

```javascript
const { test, expect } = require('@playwright/test');

test('description', async ({ page }) => {
  await page.goto('/');
  await expect(page.locator('#element')).toBeVisible();
});
```

## CI/CD

Tests can be run in CI by setting the `CI` environment variable:

```bash
CI=1 npm test
```

This will:
- Run tests with 2 retries
- Use a single worker
- Build and start the server automatically
