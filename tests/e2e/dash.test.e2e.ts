import type {Page} from '@playwright/test';
import {expect, test} from './utils_e2e.ts';

// fixture members: boss (owner), ed (editor), vi (viewer); dev sign-in is on in the harness
async function signIn(page: Page, login: string, next = '/admin') {
  await page.goto(`/signin?next=${encodeURIComponent(next)}`);
  await page.getByLabel('Логин из списка доступа').fill(login);
  await page.getByRole('button', {name: 'Войти (dev)'}).click();
  await expect(page).toHaveURL(next);
}

test('the public page lists public expenses and donations', async ({page}) => {
  await page.goto('/');
  const table = page.locator('#public-expenses');
  await expect(table).toContainText('grm');
  await expect(table).not.toContainText('secret-backup');
  await expect(page.locator('.donation-feed')).toContainText('Аноним');
  await expect(page.getByRole('link', {name: 'Админка'})).toHaveCount(0);
});

test('an editor records a payment and the due date moves', async ({page}) => {
  await signIn(page, 'ed', '/admin/expenses/2');
  await expect(page.locator('#next-due')).toContainText('2020');
  await page.locator('#p-amount').fill('1500');
  await page.locator('#note').fill('счёт из e2e');
  await page.getByRole('button', {name: 'Записать оплату'}).click();
  await expect(page.locator('#flash')).toContainText('Оплата записана');
  await expect(page.locator('#payments')).toContainText('счёт из e2e');
  await expect(page.locator('#next-due')).not.toContainText('2020');
});

test('an invalid payment shows a toast and keeps the page', async ({page}) => {
  await signIn(page, 'ed', '/admin/expenses/1');
  await page.locator('#p-amount').fill('-5');
  await page.getByRole('button', {name: 'Записать оплату'}).click();
  await expect(page.locator('#toast-container')).toContainText('отрицательной');
  await expect(page).toHaveURL('/admin/expenses/1');
});

test('an editor edits an expense from the list', async ({page}) => {
  await signIn(page, 'ed', '/admin/expenses');
  await page.locator('#expense-1').getByRole('link', {name: 'изменить'}).click();
  await page.getByLabel('Название').fill('grm (AX42)');
  await page.getByRole('button', {name: 'Сохранить'}).click();
  await expect(page.locator('#flash')).toContainText('Сохранено');
  await expect(page.locator('h1')).toContainText('grm (AX42)');
});

test('the expense list filters apply on change', async ({page}) => {
  await signIn(page, 'vi', '/admin/expenses');
  await page.getByLabel('Статус').selectOption('overdue');
  await expect(page).toHaveURL(/status=overdue/);
  await expect(page.locator('#expenses tr[id^="expense-"]')).toHaveCount(1);
});

test('an anonymous donation added by hand shows up publicly', async ({page}) => {
  await signIn(page, 'ed', '/admin/donations');
  const form = page.locator('#donation-form');
  await form.getByLabel('Имя').fill('Петя');
  await form.getByLabel('Анонимно').check();
  await expect(form.getByLabel('Имя')).toBeDisabled();
  await form.getByLabel('Сумма').fill('777');
  await form.getByRole('button', {name: 'Добавить'}).click();
  await expect(page.locator('#flash')).toContainText('Донат добавлен');
  await page.goto('/');
  await expect(page.locator('.donation-feed')).toContainText('+777');
  await expect(page.locator('.donation-feed')).not.toContainText('Петя');
});

test('a viewer sees no edit controls', async ({page}) => {
  await signIn(page, 'vi', '/admin/expenses/1');
  await expect(page.locator('#edit-expense')).toHaveCount(0);
  await expect(page.locator('#payment-form')).toHaveCount(0);
  await expect(page.getByRole('link', {name: 'Пользователи'})).toHaveCount(0);
});
