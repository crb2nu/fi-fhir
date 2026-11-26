import adapter from '@sveltejs/adapter-static';
import { sveltekit } from '@sveltejs/kit/vite';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';
import { defineConfig, loadEnv } from 'vite';

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '');
  const apiOrigin = env.VITE_API_ORIGIN ?? 'http://localhost:8081';

  return {
    // SvelteKit 3 reads its config from the plugin; svelte.config.js is rejected.
    plugins: [
      sveltekit({
        preprocess: vitePreprocess(),
        // Kit 3 replaced `$lib` with `#lib`; keep `$lib` rather than rewrite every import.
        alias: { $lib: 'src/lib' },
        adapter: adapter({
          pages: 'build',
          assets: 'build',
          fallback: 'index.html', // SPA mode - all routes handled client-side
          precompress: false,
          strict: true
        })
      })
    ],
    server: {
      proxy: {
        '/graphql': { target: apiOrigin, changeOrigin: true },
        '/health': { target: apiOrigin, changeOrigin: true },
        // Mirrors nginx/default.conf.template so the credential gate can detect
        // trusted-network / Cloudflare Access sessions against a local API.
        '/api/auth/status': { target: apiOrigin, changeOrigin: true }
      }
    }
  };
});
