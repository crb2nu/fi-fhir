/**
 * The theme switch contract tokens.css relies on: dark is the default, and the
 * *resolved* theme is always written to data-theme ("system" included), so
 * the CSS needs only `:root` (dark) and `[data-theme="light"]`.
 */
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { get } from 'svelte/store';
import {
  appliedTheme,
  DEFAULT_THEME_PREFERENCE,
  initTheme,
  setThemePreference,
  themePreference
} from './theme';

const root = document.documentElement;
const originalMatchMedia = window.matchMedia;

function mockSystemDark(dark: boolean): void {
  window.matchMedia = vi.fn().mockImplementation((query: string) => ({
    matches: dark && query.includes('dark'),
    media: query,
    onchange: null,
    addListener: () => {},
    removeListener: () => {},
    addEventListener: () => {},
    removeEventListener: () => {},
    dispatchEvent: () => false
  }));
}

describe('theme', () => {
  beforeEach(() => {
    localStorage.clear();
    root.removeAttribute('data-theme');
  });

  afterEach(() => {
    window.matchMedia = originalMatchMedia;
    setThemePreference('dark');
    localStorage.clear();
  });

  it('defaults to dark with no stored preference, even on a light OS', () => {
    mockSystemDark(false);
    initTheme();
    expect(DEFAULT_THEME_PREFERENCE).toBe('dark');
    expect(get(themePreference)).toBe('dark');
    expect(get(appliedTheme)).toBe('dark');
    expect(root.getAttribute('data-theme')).toBe('dark');
    expect(root.style.backgroundColor).toBe('rgb(20, 21, 24)'); // #141518
  });

  it('writes the resolved theme for "system"', () => {
    mockSystemDark(false);
    setThemePreference('system');
    expect(root.getAttribute('data-theme')).toBe('light');
    expect(get(appliedTheme)).toBe('light');

    mockSystemDark(true);
    setThemePreference('system');
    expect(root.getAttribute('data-theme')).toBe('dark');
  });

  it('persists explicit choices and restores them on init', () => {
    setThemePreference('light');
    expect(localStorage.getItem('fi-fhir-theme')).toBe('light');
    root.removeAttribute('data-theme');

    initTheme();
    expect(get(themePreference)).toBe('light');
    expect(root.getAttribute('data-theme')).toBe('light');
  });
});
