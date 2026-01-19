import { spawn } from 'child_process';
import { createRequire } from 'module';
import { existsSync } from 'fs';
import { isAbsolute, join, resolve } from 'path';

/**
 * Path to the fi-fhir binary. Can be overridden via FI_FHIR_PATH env var.
 */
const requireFn = (() => {
  // Prefer resolving from the consumer project root so optionalDependencies can be found
  // regardless of whether the SDK is imported as CJS or ESM.
  try {
    return createRequire(join(process.cwd(), 'package.json'));
  } catch {
    // Fallback: best-effort local require. This may still fail in REPL-like contexts.
    return createRequire(join(process.cwd(), 'index.js'));
  }
})();

function platformPackageName(): string | null {
  const { platform, arch } = process;

  if (platform === 'darwin' && arch === 'arm64') return '@fi-fhir/fi-fhir-darwin-arm64';
  if (platform === 'darwin' && arch === 'x64') return '@fi-fhir/fi-fhir-darwin-x64';

  if (platform === 'linux' && arch === 'arm64') return '@fi-fhir/fi-fhir-linux-arm64';
  if (platform === 'linux' && arch === 'x64') return '@fi-fhir/fi-fhir-linux-x64';

  if (platform === 'win32' && arch === 'x64') return '@fi-fhir/fi-fhir-win32-x64';

  return null;
}

function resolveFiFhirBinaryPath(): string {
  const envPath = process.env.FI_FHIR_PATH?.trim();
  if (envPath) {
    const baseDir = process.env.INIT_CWD || process.cwd();
    return isAbsolute(envPath) ? envPath : resolve(baseDir, envPath);
  }

  const pkgName = platformPackageName();
  if (pkgName) {
    try {
      const mod = requireFn(pkgName) as { fiFhirPath?: string };
      if (mod?.fiFhirPath && existsSync(mod.fiFhirPath)) {
        return mod.fiFhirPath;
      }
    } catch {
      // ignore; fall back to PATH lookup
    }
  }

  return 'fi-fhir';
}

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
  const binaryPath = resolveFiFhirBinaryPath();

  return new Promise((resolve, reject) => {
    const proc = spawn(binaryPath, args, {
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
          `fi-fhir binary not found at "${binaryPath}". ` +
          'Install fi-fhir (or install @fi-fhir/sdk with a supported platform binary), ' +
          'or set FI_FHIR_PATH environment variable.',
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
