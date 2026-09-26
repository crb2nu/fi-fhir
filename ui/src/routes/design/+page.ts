import { redirect } from '@sveltejs/kit';
import type { PageLoad } from './$types';

// The primitives gallery exists for development and review screenshots only.
export const load: PageLoad = () => {
  if (!import.meta.env.DEV) redirect(307, '/');
};
