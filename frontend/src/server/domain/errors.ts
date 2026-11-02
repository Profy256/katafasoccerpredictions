/**
 * Domain errors the service layer throws and the HTTP layer translates into
 * problem+json. The kind is the only thing a handler needs to pick a status,
 * so handlers never have to inspect messages.
 *
 * Port of backend/internal/domain/errors.go. The comment that matters is on
 * `notFound`: it is also what a non-admin gets for a draft slip, so that
 * unpublished slip ids are not discoverable by probing for a 403.
 */

export type ErrorKind =
  | 'not-found'
  | 'unauthorized'
  | 'forbidden'
  | 'conflict'
  | 'unprocessable'
  | 'invalid-request';

export class AppError extends Error {
  readonly kind: ErrorKind;

  constructor(kind: ErrorKind, message: string) {
    super(message);
    this.name = 'AppError';
    this.kind = kind;
  }

  static notFound(message = 'not found'): AppError {
    return new AppError('not-found', message);
  }

  static unauthorized(message = 'unauthorized'): AppError {
    return new AppError('unauthorized', message);
  }

  static forbidden(message = 'forbidden'): AppError {
    return new AppError('forbidden', message);
  }

  /** State conflicts: already purchased, slip closed, already settled. */
  static conflict(message: string): AppError {
    return new AppError('conflict', `conflict: ${message}`);
  }

  /** A well-formed request rejected on its content. */
  static unprocessable(message: string): AppError {
    return new AppError('unprocessable', `unprocessable: ${message}`);
  }

  /** A malformed body, an unknown enum value, or a bad cursor. */
  static invalid(message: string): AppError {
    return new AppError('invalid-request', `validation failed: ${message}`);
  }
}

export function isAppError(error: unknown): error is AppError {
  return error instanceof AppError;
}
