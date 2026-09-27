import js from '@eslint/js';
import globals from 'globals';
import svelte from 'eslint-plugin-svelte';
import tseslint from 'typescript-eslint';

export default [
  {
    ignores: ['.svelte-kit/**', 'build/**', 'node_modules/**', 'src/lib/gen/**']
  },

  js.configs.recommended,

  ...tseslint.configs.recommended,

  ...svelte.configs['flat/recommended'],

  {
    languageOptions: {
      globals: {
        ...globals.browser,
        ...globals.node
      }
    },
    rules: {
      'no-console': ['warn', { allow: ['warn', 'error'] }]
    }
  },

  // Svelte files, and rune modules (*.svelte.ts), which the Svelte parser
  // hands to the TypeScript parser for their script.
  {
    files: ['**/*.svelte', '**/*.svelte.ts'],
    languageOptions: {
      parserOptions: {
        parser: tseslint.parser
      }
    }
  }
];

