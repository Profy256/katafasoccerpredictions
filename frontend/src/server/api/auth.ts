import { AppError } from '@/server/domain/errors';
import type { User } from '@/server/domain/user';
import { statusResponse } from '@/server/http/render';
import type { ApiHandler } from '@/server/http/middleware';

/**
 * Session reads. Port of handleMe in backend/internal/api/handlers_auth.go —
 * the user JSON is the User struct with passwordHash on `json:"-"`.
 *
 * Register, login and logout arrive with the paywall phase; this file grows
 * with them.
 */

function userJSON(user: User): Record<string, unknown> {
  // Key order is Go's struct order: id, email, name, phone?, role, createdAt.
  const out: Record<string, unknown> = {
    id: user.id,
    email: user.email,
    name: user.name,
  };
  // phone is a pointer with omitempty — absent when null.
  if (user.phone !== null) out.phone = user.phone;
  out.role = user.role;
  out.createdAt = user.createdAt;
  return out;
}

export const handleMe: ApiHandler = async (ctx) => {
  if (!ctx.user) throw AppError.unauthorized();
  // Go's render.Status: an explicit 200, no-store, no ETag, trailing newline.
  return statusResponse(200, { user: userJSON(ctx.user) });
};
