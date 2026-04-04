/**
 * Platform configuration from environment variables.
 * When PUBLIC_LOOM_ENDPOINT is not set, platform features are disabled.
 */
export interface PlatformConfig {
  endpoint: string;
  token: string;
  agentId: string;
  enabled: boolean;
}

export const PLATFORM_CONFIG: PlatformConfig = {
  endpoint: import.meta.env.PUBLIC_LOOM_ENDPOINT || 'http://localhost:8080/mcp',
  token: import.meta.env.PUBLIC_LOOM_TOKEN || '',
  agentId: 'mapping-studio',
  enabled: !!import.meta.env.PUBLIC_LOOM_ENDPOINT,
};
