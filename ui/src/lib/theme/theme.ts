import { browser } from '$app/environment';
import { writable } from 'svelte/store';

export type ThemePreference = 'system' | 'light' | 'dark';
export type AppliedTheme = 'light' | 'dark';

// Keep this in sync with src/app.html early-init script.
const STORAGE_KEY = 'fi-fhir-theme';
const THEME_ATTR = 'data-theme';
const DARK_MQ = '(prefers-color-scheme: dark)';

const LIGHT_BG = '#f8fafc';
const DARK_BG = '#0b1220';

export const themePreference = writable<ThemePreference>('system');
export const appliedTheme = writable<AppliedTheme>('light');

let mq: MediaQueryList | null = null;
let mqCleanup: (() => void) | null = null;

function getStoredPreference(): ThemePreference {
  if (!browser) return 'system';
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (raw === 'light' || raw === 'dark' || raw === 'system') return raw;
  } catch {
    // ignore
  }
  return 'system';
}

function setStoredPreference(pref: ThemePreference): void {
  try {
    localStorage.setItem(STORAGE_KEY, pref);
  } catch {
    // ignore
  }
}

function resolveApplied(pref: ThemePreference): AppliedTheme {
  if (pref === 'dark' || pref === 'light') return pref;
  if (!browser) return 'light';
  return matchMedia(DARK_MQ).matches ? 'dark' : 'light';
}

function setThemeColorMeta(theme: AppliedTheme): void {
  const meta = document.querySelector('meta[name="theme-color"]');
  if (!meta) return;
  meta.setAttribute('content', theme === 'dark' ? DARK_BG : LIGHT_BG);
}

function applyToDom(pref: ThemePreference, theme: AppliedTheme): void {
  const root = document.documentElement;

  // Persisted override: data-theme="light|dark". System preference: no attribute.
  if (pref === 'dark' || pref === 'light') root.setAttribute(THEME_ATTR, pref);
  else root.removeAttribute(THEME_ATTR);

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
    const theme = resolveApplied('system');
    appliedTheme.set(theme);
    applyToDom('system', theme);
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
  const theme = resolveApplied(pref);
  appliedTheme.set(theme);
  applyToDom(pref, theme);

  if (pref === 'system') attachSystemListener();
  else detachSystemListener();
}

export function setThemePreference(pref: ThemePreference): void {
  if (!browser) return;
  themePreference.set(pref);
  setStoredPreference(pref);

  const theme = resolveApplied(pref);
  appliedTheme.set(theme);
  applyToDom(pref, theme);

  if (pref === 'system') attachSystemListener();
  else detachSystemListener();
}

