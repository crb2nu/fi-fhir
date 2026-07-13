const PROFILE_YAML_UNAVAILABLE =
  'Profile YAML transport is unavailable during authenticated preview hardening';

export async function saveProfileYaml(profileId: string, yaml: string): Promise<void> {
  void profileId;
  void yaml;
  throw new Error(PROFILE_YAML_UNAVAILABLE);
}

export async function fetchProfileYaml(profileId: string): Promise<string> {
  void profileId;
  throw new Error(PROFILE_YAML_UNAVAILABLE);
}
