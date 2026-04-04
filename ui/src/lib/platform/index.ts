export { PLATFORM_CONFIG, type PlatformConfig } from './config';
export { createMcpClient, type McpClient } from './mcpClient';
export { platformState, initializePlatform, teardownPlatform, type PlatformState } from './platformStore';
export { startOperatorSession, endOperatorSession, addContextEntry, type ContextEntry } from './sessionManager';
