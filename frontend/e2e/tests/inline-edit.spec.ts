import { test, expect, type Page } from '@playwright/test';
import { CaseListPage } from '../pages/CaseListPage';
import { CaseFormPage } from '../pages/CaseFormPage';
import { CaseDetailPage } from '../pages/CaseDetailPage';
import { TEST_WORKSPACE_ID } from '../fixtures/testData';

// Asserts that no GraphQL response since `start()` carried an `errors` payload.
// The previous bug shape was a 200 OK with `errors:[{message:"... validation
// failed ..."}]`, which a quick visual smoke test would miss.
function graphqlErrorWatcher(page: Page) {
  const failures: string[] = [];
  const handler = async (response: import('@playwright/test').Response) => {
    if (!response.url().includes('/graphql')) return;
    if (response.request().method() !== 'POST') return;
    try {
      const body = await response.json();
      if (body && Array.isArray(body.errors) && body.errors.length > 0) {
        failures.push(body.errors.map((e: any) => e.message).join(' | '));
      }
    } catch {
      // Non-JSON or stream already consumed — ignore.
    }
  };
  page.on('response', handler);
  return {
    failures,
    stop: () => page.off('response', handler),
  };
}

// The shared E2E config (config.test.toml) exposes:
//   - SELECT (category, priority)
//   - TEXT   (description)
// We exercise inline-edit on each of these. NUMBER / DATE / URL /
// MULTI_SELECT / USER / MULTI_USER are validated by Go-level tests
// (validateNumber, etc.) plus the live "risk" config; covering them here
// would require a wider field set in config.test.toml, which makes the
// New-Case modal taller than the viewport and breaks unrelated tests.
test.describe('Inline edit — covered field types', () => {
  test.beforeEach(async ({ page }) => {
    const caseListPage = new CaseListPage(page);
    await caseListPage.navigate(TEST_WORKSPACE_ID);
    await caseListPage.waitForTableLoad();
  });

  test('inline edits persist after reload', async ({ page }) => {
    const caseListPage = new CaseListPage(page);
    const caseFormPage = new CaseFormPage(page);
    const caseDetailPage = new CaseDetailPage(page);

    // Seed a case so we can edit its custom fields inline.
    await caseListPage.clickNewCaseButton();
    await caseFormPage.createCase({
      title: 'Inline edit field coverage',
      description: 'Used by inline-edit.spec',
      customFields: { category: 'bug' },
    });
    await caseListPage.waitForTableLoad();
    await caseListPage.fillSearchFilter('Inline edit field coverage');
    await caseListPage.clickCaseByTitle('Inline edit field coverage');
    expect(await caseDetailPage.isPageLoaded()).toBeTruthy();

    const watcher = graphqlErrorWatcher(page);

    // Match the UpdateCase mutation specifically — UPDATE_CASE has
    // `refetchQueries`, so several /graphql POSTs follow each edit and a
    // generic predicate could resolve on a refetch instead of the mutation.
    const isUpdateCaseResponse = (r: import('@playwright/test').Response) => {
      if (!r.url().includes('/graphql') || r.request().method() !== 'POST') return false;
      const body = r.request().postDataJSON?.();
      return body?.operationName === 'UpdateCase';
    };

    // SELECT — change category from bug → feature.
    await page.getByTestId('field-category').click();
    const categoryResp = page.waitForResponse(isUpdateCaseResponse);
    await page.getByTestId('field-category-option-feature').click();
    await categoryResp;

    // SELECT (newly set) — priority high.
    await page.getByTestId('field-priority').click();
    const priorityResp = page.waitForResponse(isUpdateCaseResponse);
    await page.getByTestId('field-priority-option-high').click();
    await priorityResp;

    // TEXT — custom-field "description".
    await page.getByTestId('field-description').click();
    const textInput = page.getByTestId('field-description-input');
    await textInput.fill('hello world');
    const descriptionResp = page.waitForResponse(isUpdateCaseResponse);
    await textInput.press('Enter');
    await descriptionResp;

    watcher.stop();
    expect(watcher.failures, `GraphQL responses returned errors: ${watcher.failures.join('\n')}`).toEqual([]);

    // Persist check: reload and verify values are still present.
    await page.reload();
    await caseDetailPage.waitForPageLoad();
    await expect(page.getByTestId('field-category')).toContainText('Feature');
    await expect(page.getByTestId('field-priority')).toContainText('High');
    await expect(page.getByTestId('field-description')).toContainText('hello world');
  });
});

// extra01 defines `notify_channel` as a text field with
// semantic = "slack_channel_id". The E2E backend has no Slack, so the server
// cannot resolve a channel name and reports every value as unresolved.
test.describe('Inline edit — text field semantic', () => {
  const SEMANTIC_WORKSPACE_ID = 'extra01';

  test('rejects a channel name, stores a channel ID, and says its name could not be resolved', async ({ page }) => {
    const caseListPage = new CaseListPage(page);
    const caseFormPage = new CaseFormPage(page);
    const caseDetailPage = new CaseDetailPage(page);
    const title = `Semantic channel ${Date.now()}`;

    await caseListPage.navigate(SEMANTIC_WORKSPACE_ID);
    await caseListPage.waitForTableLoad();
    await caseListPage.clickNewCaseButton();
    await caseFormPage.createCase({ title, description: 'Used by inline-edit.spec semantic test' });
    await caseListPage.waitForTableLoad();
    await caseListPage.fillSearchFilter(title);
    await caseListPage.clickCaseByTitle(title);
    expect(await caseDetailPage.isPageLoaded()).toBeTruthy();

    const isUpdateCaseResponse = (r: import('@playwright/test').Response) => {
      if (!r.url().includes('/graphql') || r.request().method() !== 'POST') return false;
      const body = r.request().postDataJSON?.();
      return body?.operationName === 'UpdateCase';
    };

    // A channel name breaks the semantic: the server rejects the write and
    // the editor stays open for a retry.
    await page.getByTestId('field-notify_channel').click();
    const input = page.getByTestId('field-notify_channel-input');
    await input.fill('#general');
    const rejected = page.waitForResponse(isUpdateCaseResponse);
    await input.press('Enter');
    const rejectedBody = await (await rejected).json();
    expect(Array.isArray(rejectedBody.errors) && rejectedBody.errors.length > 0).toBeTruthy();
    await expect(input).toBeVisible();

    // A channel ID is stored.
    await input.fill('C0123ABCD');
    const accepted = page.waitForResponse(isUpdateCaseResponse);
    await input.press('Enter');
    const acceptedBody = await (await accepted).json();
    expect(acceptedBody.errors ?? []).toEqual([]);

    // After a reload the value is shown as stored. With no Slack the name
    // cannot be resolved, so the line under it says so and links nowhere.
    await page.reload();
    await caseDetailPage.waitForPageLoad();
    await expect(page.getByTestId('field-notify_channel')).toContainText('C0123ABCD');
    const supplement = page.getByTestId('field-notify_channel-supplement');
    await expect(supplement).toContainText("Couldn't resolve the name");
    await expect(supplement.locator('a')).toHaveCount(0);

    // With a resolved name (the GraphQL display rewritten to what a Slack
    // workspace would return), the ID stays as the value and the channel
    // name is shown under it, linked to the channel.
    await page.route('**/graphql', async (route) => {
      const body = route.request().postDataJSON?.();
      if (body?.operationName !== 'GetCase') return route.continue();
      const response = await route.fetch();
      const json = await response.json();
      for (const f of json?.data?.case?.fields ?? []) {
        if (f.fieldId === 'notify_channel') {
          f.display = { label: '#general', url: 'https://slack.com/archives/C0123ABCD' };
        }
      }
      await route.fulfill({ response, json });
    });
    await page.reload();
    await caseDetailPage.waitForPageLoad();
    await expect(page.getByTestId('field-notify_channel')).toContainText('C0123ABCD');
    const nameLink = supplement.locator('a');
    await expect(nameLink).toHaveText('#general');
    await expect(nameLink).toHaveAttribute('href', 'https://slack.com/archives/C0123ABCD');
    await expect(nameLink).toHaveAttribute('target', '_blank');
  });
});
