/**
 * The loom platform is optional chrome: with no PUBLIC_LOOM_ENDPOINT the shell
 * neither starts the platform client nor shows its status-bar indicator.
 */
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import { tick } from 'svelte';
import { writable } from 'svelte/store';
import { resetIDEState } from './ideStore';

const pageStore = writable({ url: new URL('http://localhost/') });

const platform = vi.hoisted(() => ({
  config: { endpoint: '', token: '', agentId: 'mapping-studio', enabled: false },
  initializePlatform: vi.fn(async () => {}),
  teardownPlatform: vi.fn(async () => {})
}));

vi.mock('$app/stores', () => ({ page: pageStore }));
vi.mock('$app/navigation', () => ({ goto: vi.fn() }));
vi.mock('$app/paths', () => ({ resolve: (path: string) => path }));
vi.mock('$lib/platform', async () => {
  const { writable: storeOf } = await import('svelte/store');
  return {
    PLATFORM_CONFIG: platform.config,
    platformState: storeOf({ connected: false }),
    initializePlatform: platform.initializePlatform,
    teardownPlatform: platform.teardownPlatform
  };
});

const { default: IDEShell } = await import('./IDEShell.svelte');

describe('IDEShell platform chrome', () => {
  beforeEach(() => {
    resetIDEState();
    platform.initializePlatform.mockClear();
    platform.teardownPlatform.mockClear();
  });

  it('does not start the platform client or show its indicator when unconfigured', async () => {
    platform.config.enabled = false;
    const { unmount } = render(IDEShell);
    await tick();

    expect(platform.initializePlatform).not.toHaveBeenCalled();
    expect(screen.queryByTestId('platform-indicator')).not.toBeInTheDocument();

    unmount();
    expect(platform.teardownPlatform).not.toHaveBeenCalled();
  });

  it('starts the platform client and shows the indicator when an endpoint is configured', async () => {
    platform.config.enabled = true;
    try {
      render(IDEShell);
      await tick();

      expect(platform.initializePlatform).toHaveBeenCalledTimes(1);
      expect(screen.getByTestId('platform-indicator')).toBeInTheDocument();
    } finally {
      platform.config.enabled = false;
    }
  });
});
