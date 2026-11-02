/**
 * The `users` row as the API sees it. Port of backend/internal/domain User —
 * `passwordHash` carries `json:"-"` in Go and is never serialized from here.
 */

export type Role = 'analyst' | 'admin';

export interface User {
  id: string;
  email: string;
  passwordHash: string;
  name: string;
  phone: string | null;
  role: Role;
  createdAt: string;
}
