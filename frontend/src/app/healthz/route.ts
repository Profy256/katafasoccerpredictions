import { statusResponse } from '@/server/http/render';

export const dynamic = 'force-dynamic';

export function GET(): Response {
  return statusResponse(200, { status: 'ok' });
}
