/**
 * Vitest setup file - runs before all tests
 */
import '@testing-library/jest-dom/vitest';

const localStorageMemory = (() => {
  const data = new Map<string, string>();

  return {
    getItem(key: string): string | null {
      return data.has(key) ? data.get(key) ?? null : null;
    },
    setItem(key: string, value: string): void {
      data.set(key, String(value));
    },
    removeItem(key: string): void {
      data.delete(key);
    },
    clear(): void {
      data.clear();
    },
    key(index: number): string | null {
      return Array.from(data.keys())[index] ?? null;
    },
    get length(): number {
      return data.size;
    },
  };
})();

Object.defineProperty(window, 'localStorage', {
  configurable: true,
  value: localStorageMemory,
});

// Mock browser APIs not available in jsdom
Object.defineProperty(window, 'matchMedia', {
  writable: true,
  value: (query: string) => ({
    matches: false,
    media: query,
    onchange: null,
    addListener: () => {},
    removeListener: () => {},
    addEventListener: () => {},
    removeEventListener: () => {},
    dispatchEvent: () => false
  })
});

// Mock ResizeObserver
class ResizeObserverMock {
  observe() {}
  unobserve() {}
  disconnect() {}
}
window.ResizeObserver = ResizeObserverMock;

// Mock IntersectionObserver
class IntersectionObserverMock {
  constructor() {}
  observe() {}
  unobserve() {}
  disconnect() {}
}
window.IntersectionObserver = IntersectionObserverMock as unknown as typeof IntersectionObserver;
