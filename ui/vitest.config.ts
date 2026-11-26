import { defineConfig } from 'vitest/config';
import { svelte } from '@sveltejs/vite-plugin-svelte';

export default defineConfig({
  plugins: [svelte()],
  test: {
    include: ['src/**/*.{test,spec}.{js,ts}'],
    globals: true,
    environment: 'jsdom',
    setupFiles: ['./src/test/setup.ts'],
    // Resolve SvelteKit aliases
    alias: {
      $lib: '/src/lib',
      $app: '/src/test/mocks/app'
    }
  },
  resolve: {
    conditions: ['browser']
  }
});
