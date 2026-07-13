import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig, loadEnv } from 'vite';

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '');
  const apiOrigin = env.VITE_API_ORIGIN ?? 'http://localhost:8081';

  return {
    plugins: [sveltekit()],
    server: {
      proxy: {
        '/graphql': { target: apiOrigin, changeOrigin: true },
        '/health': { target: apiOrigin, changeOrigin: true }
      }
    }
  };
});
