export async function saveProfileYaml(profileId: string, yaml: string): Promise<void> {
  const res = await fetch(`/api/profiles/${encodeURIComponent(profileId)}/yaml`, {
    method: 'PUT',
    headers: { 'content-type': 'text/yaml' },
    body: yaml
  });

  if (!res.ok) {
    throw new Error(`Profile YAML save failed (HTTP ${res.status})`);
  }
}

export async function fetchProfileYaml(profileId: string): Promise<string> {
  const res = await fetch(`/api/profiles/${encodeURIComponent(profileId)}/yaml`, {
    method: 'GET',
    headers: { accept: 'text/yaml' }
  });

  if (!res.ok) {
    throw new Error(`Profile YAML fetch failed (HTTP ${res.status})`);
  }

  return res.text();
}

