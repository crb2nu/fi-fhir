import { execFiFhir, FiFhirError } from './utils/cli';
import type { HealthcareEvent } from './types/events';

/**
 * Result from workflow execution
 */
export interface WorkflowResult {
  eventsProcessed: number;
  routeMatches: number;
  errors: number;
}

/**
 * Result from workflow dry-run
 */
export interface DryRunResult {
  eventIndex: number;
  routes: RouteMatchResult[];
}

/**
 * Route match result from dry-run
 */
export interface RouteMatchResult {
  name: string;
  matched: boolean;
  actionsCount?: number;
}

/**
 * Workflow validation result
 */
export interface ValidationResult {
  valid: boolean;
  name?: string;
  version?: string;
  routes?: RouteInfo[];
  errors?: string[];
}

/**
 * Route information from validation
 */
export interface RouteInfo {
  name: string;
  actionsCount: number;
}

/**
 * Workflow class for processing events through configured routes
 *
 * @example
 * ```typescript
 * const workflow = new Workflow('./workflow.yaml');
 *
 * // Validate configuration
 * const validation = await workflow.validate();
 * if (!validation.valid) {
 *   console.error(validation.errors);
 * }
 *
 * // Process events
 * const result = await workflow.run(events);
 * console.log(`Processed ${result.eventsProcessed} events`);
 * ```
 */
export class Workflow {
  private configPath: string;

  /**
   * Create a workflow instance
   * @param configPath - Path to workflow YAML configuration file
   */
  constructor(configPath: string) {
    this.configPath = configPath;
  }

  /**
   * Load a workflow from a YAML file
   * @param configPath - Path to workflow configuration
   */
  static load(configPath: string): Workflow {
    return new Workflow(configPath);
  }

  /**
   * Validate the workflow configuration
   * @returns Validation result with route information
   */
  async validate(): Promise<ValidationResult> {
    try {
      const { stdout } = await execFiFhir([
        'workflow', 'validate', this.configPath
      ]);

      // Parse validation output
      const lines = stdout.trim().split('\n');
      const result: ValidationResult = { valid: true };

      for (const line of lines) {
        const nameMatch = line.match(/Workflow '(.+)' is valid/);
        if (nameMatch) {
          result.name = nameMatch[1];
        }

        const versionMatch = line.match(/Version: (.+)/);
        if (versionMatch) {
          result.version = versionMatch[1];
        }

        const routeMatch = line.match(/- (.+) \((\d+) actions?\)/);
        if (routeMatch) {
          result.routes = result.routes || [];
          result.routes.push({
            name: routeMatch[1],
            actionsCount: parseInt(routeMatch[2], 10)
          });
        }
      }

      return result;
    } catch (error) {
      if (error instanceof FiFhirError) {
        return {
          valid: false,
          errors: [error.message]
        };
      }
      throw error;
    }
  }

  /**
   * Process events through the workflow
   * @param events - Array of events or JSON string
   * @param options - Execution options
   */
  async run(
    events: HealthcareEvent[] | string,
    options: { timeout?: number } = {}
  ): Promise<WorkflowResult> {
    const input = typeof events === 'string'
      ? events
      : JSON.stringify(events);

    const { stdout } = await execFiFhir(
      ['workflow', 'run', '-c', this.configPath, '-'],
      input,
      { timeout: options.timeout }
    );

    // Parse result: "Processed N events, M route matches, E errors"
    const match = stdout.match(/Processed (\d+) events?, (\d+) route matches?, (\d+) errors?/);
    if (match) {
      return {
        eventsProcessed: parseInt(match[1], 10),
        routeMatches: parseInt(match[2], 10),
        errors: parseInt(match[3], 10)
      };
    }

    return { eventsProcessed: 0, routeMatches: 0, errors: 0 };
  }

  /**
   * Dry-run events through the workflow without executing actions
   * @param events - Array of events or JSON string
   */
  async dryRun(
    events: HealthcareEvent[] | string,
    options: { timeout?: number } = {}
  ): Promise<DryRunResult[]> {
    const input = typeof events === 'string'
      ? events
      : JSON.stringify(events);

    const { stdout } = await execFiFhir(
      ['workflow', 'dry-run', '-c', this.configPath, '-'],
      input,
      { timeout: options.timeout }
    );

    // Parse dry-run output
    const results: DryRunResult[] = [];
    let currentEvent: DryRunResult | null = null;

    for (const line of stdout.split('\n')) {
      const eventMatch = line.match(/Event (\d+):/);
      if (eventMatch) {
        if (currentEvent) {
          results.push(currentEvent);
        }
        currentEvent = {
          eventIndex: parseInt(eventMatch[1], 10),
          routes: []
        };
        continue;
      }

      if (currentEvent) {
        const routeMatch = line.match(/Route '(.+)': (MATCH|NO MATCH)(?: - would run (\d+) action)?/);
        if (routeMatch) {
          currentEvent.routes.push({
            name: routeMatch[1],
            matched: routeMatch[2] === 'MATCH',
            actionsCount: routeMatch[3] ? parseInt(routeMatch[3], 10) : undefined
          });
        }
      }
    }

    if (currentEvent) {
      results.push(currentEvent);
    }

    return results;
  }
}

// Re-export error type
export { FiFhirError };
