import { browser } from '$app/environment';
import { writable } from 'svelte/store';

export type ThemePreference = 'system' | 'light' | 'dark';
export type AppliedTheme = 'light' | 'dark';

// Keep this in sync with src/app.html early-init script.
const STORAGE_KEY = 'fi-fhir-theme';
const THEME_ATTR = 'data-theme';
const DARK_MQ = '(prefers-color-scheme: dark)';

/** Dark is the default workbench theme; "system" is an explicit choice. */
export const DEFAULT_THEME_PREFERENCE: ThemePreference = 'dark';

// Mirror --color-bg-base in src/lib/styles/tokens.css.
const LIGHT_BG = '#f6f7f9';
const DARK_BG = '#141518';

export const themePreference = writable<ThemePreference>(DEFAULT_THEME_PREFERENCE);
export const appliedTheme = writable<AppliedTheme>('dark');

let mq: MediaQueryList | null = null;
let mqCleanup: (() => void) | null = null;

function getStoredPreference(): ThemePreference {
  if (!browser) return DEFAULT_THEME_PREFERENCE;
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (raw === 'light' || raw === 'dark' || raw === 'system') return raw;
  } catch {
    // ignore
  }
  return DEFAULT_THEME_PREFERENCE;
}

function setStoredPreference(pref: ThemePreference): void {
  try {
    localStorage.setItem(STORAGE_KEY, pref);
  } catch {
    // ignore
  }
}

export function resolveTheme(pref: ThemePreference): AppliedTheme {
  if (pref === 'dark' || pref === 'light') return pref;
  if (!browser) return 'dark';
  return matchMedia(DARK_MQ).matches ? 'dark' : 'light';
}

function setThemeColorMeta(theme: AppliedTheme): void {
  const meta = document.querySelector('meta[name="theme-color"]');
  if (!meta) return;
  meta.setAttribute('content', theme === 'dark' ? DARK_BG : LIGHT_BG);
}

function applyToDom(theme: AppliedTheme): void {
  const root = document.documentElement;

  // Always the resolved theme: tokens.css is dark on :root and overrides only
  // under [data-theme="light"], so "system" works by writing light|dark here.
  root.setAttribute(THEME_ATTR, theme);

  // Ensure the browser's first-paint background stays in sync.
  root.style.backgroundColor = theme === 'dark' ? DARK_BG : LIGHT_BG;

  setThemeColorMeta(theme);
}

function attachSystemListener(): void {
  if (!browser) return;
  if (mqCleanup) return;
  mq = matchMedia(DARK_MQ);
  const onChange = () => {
    const currentPref = getStoredPreference();
    if (currentPref !== 'system') return;
    const theme = resolveTheme('system');
    appliedTheme.set(theme);
    applyToDom(theme);
  };
  mq.addEventListener('change', onChange);
  mqCleanup = () => mq?.removeEventListener('change', onChange);
}

function detachSystemListener(): void {
  mqCleanup?.();
  mqCleanup = null;
  mq = null;
}

export function initTheme(): void {
  if (!browser) return;
  const pref = getStoredPreference();
  themePreference.set(pref);
  const theme = resolveTheme(pref);
  appliedTheme.set(theme);
  applyToDom(theme);

  if (pref === 'system') attachSystemListener();
  else detachSystemListener();
}

export function setThemePreference(pref: ThemePreference): void {
  if (!browser) return;
  themePreference.set(pref);
  setStoredPreference(pref);

  const theme = resolveTheme(pref);
  appliedTheme.set(theme);
  applyToDom(theme);

  if (pref === 'system') attachSystemListener();
  else detachSystemListener();
}
