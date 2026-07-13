import { beforeEach, describe, expect, it, vi } from 'vitest';
import { fetchProfileYaml, saveProfileYaml } from './profileYamlApi';

describe('profile YAML transport containment', () => {
  beforeEach(() => vi.stubGlobal('fetch', vi.fn()));

  it('fails locally without issuing legacy GET or PUT requests', async () => {
    await expect(fetchProfileYaml('profile-1')).rejects.toThrow(
      'Profile YAML transport is unavailable during authenticated preview hardening'
    );
    await expect(saveProfileYaml('profile-1', 'id: profile-1')).rejects.toThrow(
      'Profile YAML transport is unavailable during authenticated preview hardening'
    );
    expect(fetch).not.toHaveBeenCalled();
  });
});
