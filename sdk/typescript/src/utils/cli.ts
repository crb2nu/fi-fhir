import { spawn } from 'child_process';

/**
 * Path to the fi-fhir binary. Can be overridden via FI_FHIR_PATH env var.
 */
const BINARY_PATH = process.env.FI_FHIR_PATH || 'fi-fhir';

/**
 * Result from executing fi-fhir CLI
 */
export interface ExecResult {
  stdout: string;
  stderr: string;
  exitCode: number;
}

/**
 * Error thrown when fi-fhir execution fails
 */
export class FiFhirError extends Error {
  constructor(
    message: string,
    public exitCode: number,
    public stderr: string
  ) {
    super(message);
    this.name = 'FiFhirError';
  }
}

/**
 * Execute fi-fhir CLI command
 *
 * @param args - Command line arguments
 * @param input - Optional stdin input
 * @param options - Spawn options
 * @returns Promise with stdout and stderr
 */
export async function execFiFhir(
  args: string[],
  input?: string,
  options: { timeout?: number } = {}
): Promise<ExecResult> {
  const timeout = options.timeout ?? 30000;

  return new Promise((resolve, reject) => {
    const proc = spawn(BINARY_PATH, args, {
      stdio: ['pipe', 'pipe', 'pipe'],
      timeout,
    });

    let stdout = '';
    let stderr = '';
    let killed = false;

    proc.stdout.on('data', (data: Buffer) => {
      stdout += data.toString();
    });

    proc.stderr.on('data', (data: Buffer) => {
      stderr += data.toString();
    });

    proc.on('error', (error: NodeJS.ErrnoException) => {
      if (error.code === 'ENOENT') {
        reject(new FiFhirError(
          `fi-fhir binary not found at "${BINARY_PATH}". ` +
          'Make sure fi-fhir is installed and in your PATH, or set FI_FHIR_PATH environment variable.',
          -1,
          ''
        ));
      } else {
        reject(new FiFhirError(
          `Failed to execute fi-fhir: ${error.message}`,
          -1,
          ''
        ));
      }
    });

    proc.on('close', (code: number | null) => {
      const exitCode = code ?? (killed ? 124 : -1);

      if (exitCode === 0) {
        resolve({ stdout, stderr, exitCode });
      } else {
        reject(new FiFhirError(
          `fi-fhir exited with code ${exitCode}: ${stderr.trim() || 'Unknown error'}`,
          exitCode,
          stderr
        ));
      }
    });

    // Handle timeout
    const timeoutId = setTimeout(() => {
      killed = true;
      proc.kill('SIGTERM');
    }, timeout);

    proc.on('close', () => {
      clearTimeout(timeoutId);
    });

    // Write input to stdin if provided
    if (input !== undefined) {
      proc.stdin.write(input);
      proc.stdin.end();
    } else {
      proc.stdin.end();
    }
  });
}

/**
 * Check if fi-fhir binary is available
 */
export async function isFiFhirAvailable(): Promise<boolean> {
  try {
    await execFiFhir(['version'], undefined, { timeout: 5000 });
    return true;
  } catch {
    return false;
  }
}

/**
 * Get fi-fhir version
 */
export async function getFiFhirVersion(): Promise<string> {
  const { stdout } = await execFiFhir(['version']);
  const match = stdout.match(/fi-fhir version ([\d.]+)/);
  return match ? match[1] : 'unknown';
}
