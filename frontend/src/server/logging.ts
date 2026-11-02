/**
 * Structured logging. One JSON object per line so Vercel's log drain can parse
 * it without a regex, and so a request id in the output can be grepped.
 *
 * Port of backend/internal/logging — the levels and the request-id binding
 * match; the output format is JSON lines instead of logfmt because this is
 * where the logs will be read from a web UI, not `tail -f`.
 */

export type LogLevel = 'debug' | 'info' | 'warn' | 'error';

const LEVEL_ORDER: Record<LogLevel, number> = { debug: 10, info: 20, warn: 30, error: 40 };

export interface Logger {
  debug(message: string, fields?: Record<string, unknown>): void;
  info(message: string, fields?: Record<string, unknown>): void;
  warn(message: string, fields?: Record<string, unknown>): void;
  error(message: string, fields?: Record<string, unknown>): void;
  child(fields: Record<string, unknown>): Logger;
}

function serializeValue(value: unknown): unknown {
  if (value instanceof Error) {
    return { name: value.name, message: value.message, stack: value.stack };
  }
  return value;
}

function threshold(): number {
  const level = (process.env.LOG_LEVEL ?? 'info') as LogLevel;
  return LEVEL_ORDER[level] ?? LEVEL_ORDER.info;
}

function write(level: LogLevel, message: string, fields: Record<string, unknown>): void {
  if (LEVEL_ORDER[level] < threshold()) return;
  const line = JSON.stringify({
    time: new Date().toISOString(),
    level,
    msg: message,
    ...Object.fromEntries(Object.entries(fields).map(([k, v]) => [k, serializeValue(v)])),
  });
  if (level === 'error') {
    console.error(line);
  } else {
    console.log(line);
  }
}

export function createLogger(base: Record<string, unknown> = {}): Logger {
  const log: Logger = {
    debug: (message, fields) => write('debug', message, { ...base, ...fields }),
    info: (message, fields) => write('info', message, { ...base, ...fields }),
    warn: (message, fields) => write('warn', message, { ...base, ...fields }),
    error: (message, fields) => write('error', message, { ...base, ...fields }),
    child: (fields) => createLogger({ ...base, ...fields }),
  };
  return log;
}
