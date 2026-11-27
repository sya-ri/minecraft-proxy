import { expect, test, type Page } from '@playwright/test';
import type { RelayState } from '../src/runtime';

const base: RelayState = {
  Config: {
    Project: 'test-relay', Zone: 'asia-northeast1-a', Network: 'default', Domain: 'mc.example.com',
    CloudflareZone: 'example.com', TCPPort: 25565, UDPPort: 24454, Hours: 4,
    GoogleClientID: 'google-desktop-client', CloudflareClientID: 'cloudflare-desktop-client',
  },
  Phase: 'idle',
  Problem: '',
  Deadline: '',
  GoogleReady: true,
  CloudflareReady: true,
  ActiveProfile: 'first',
  Profiles: [
    { ID: 'first', Domain: 'mc.example.com', Phase: 'idle' },
    { ID: 'second', Domain: 'other.example.com', Phase: 'idle' },
  ],
};

async function open(page: Page, override: Partial<RelayState> = {}) {
  await page.addInitScript((initial) => {
    const first: RelayState = structuredClone(initial);
    const second: RelayState = {
      ...structuredClone(initial), Phase: 'idle', Deadline: '', Problem: '',
      Config: { ...initial.Config, Domain: 'other.example.com', TCPPort: 25566, UDPPort: 0 },
      ActiveProfile: 'second',
    };
    let selected = first;
    window.__relayTest = {
      getState: async () => ({
        ...selected, Profiles: [
          { ID: 'first', Domain: first.Config.Domain, Phase: first.Phase },
          { ID: 'second', Domain: second.Config.Domain, Phase: second.Phase },
        ],
      }),
      addProfile: async () => {},
      selectProfile: async (id) => { selected = id === 'first' ? first : second; },
      saveConfig: async () => {},
      logIn: async () => { throw new Error('Sign-in expired.'); },
      start: async () => { selected.Phase = 'running'; },
      stop: async () => { selected.Phase = 'idle'; selected.Deadline = ''; },
      extend: async () => { selected.Deadline = '2026-11-27T20:00:00Z'; },
      copy: async () => {},
    };
  }, { ...base, ...override });
  await page.goto('/');
  await expect(page.getByRole('heading', { name: 'mc.example.com' })).toBeVisible();
}

test('stopped relay', async ({ page }) => {
  await open(page);
  await expect(page.getByRole('button', { name: 'Start', exact: true })).toBeEnabled();
  await expect(page.locator('body')).toMatchAriaSnapshot({ name: 'stopped.aria.yml' });
});

test('starting relay', async ({ page }) => {
  await open(page, { Phase: 'starting' });
  await expect(page.locator('body')).toMatchAriaSnapshot({ name: 'starting.aria.yml' });
});

test('online relay and independent profile controls', async ({ page }) => {
  await open(page, { Phase: 'running', Deadline: '2026-11-27T16:00:00Z' });
  await expect(page.locator('body')).toMatchAriaSnapshot({ name: 'online.aria.yml' });
  await page.getByRole('button', { name: 'Extend 4 h' }).click();
  await expect(page.getByText('11/27/26, 8:00 PM')).toBeVisible();
  await page.getByRole('combobox', { name: 'Relay profile' }).selectOption('second');
  await expect(page.getByRole('heading', { name: 'other.example.com' })).toBeVisible();
  await page.getByRole('button', { name: 'Start', exact: true }).click();
  await expect(page.getByRole('status')).toHaveText('Online');
  await page.getByRole('button', { name: 'Stop', exact: true }).click();
  await expect(page.getByRole('status')).toHaveText('Stopped');
  await page.getByRole('combobox', { name: 'Relay profile' }).selectOption('first');
  await expect(page.getByRole('status')).toHaveText('Online');
  await expect(page.getByText('11/27/26, 8:00 PM')).toBeVisible();
});

test('failed relay', async ({ page }) => {
  await open(page, { Phase: 'recoverable', Problem: 'No Minecraft status received.' });
  await expect(page.locator('body')).toMatchAriaSnapshot({ name: 'error.aria.yml' });
});

test('settings and sign-in error', async ({ page }) => {
  await open(page);
  await page.getByRole('button', { name: 'Settings' }).click();
  const dialog = page.getByRole('dialog', { name: 'Settings' });
  await expect(dialog).toMatchAriaSnapshot({ name: 'settings.aria.yml' });
  await dialog.getByRole('button', { name: 'Sign in to Google' }).click();
  await expect(dialog.getByRole('alert')).toHaveText('Sign-in expired.');
});
