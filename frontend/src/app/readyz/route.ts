import { pingDatabase } from '@/server/db';
import { statusResponse } from '@/server/http/render';

export const dynamic = 'force-dynamic';

/**
 * Reports that this instance can serve: Postgres is reachable and the schema
 * is present. An API serving against a half-migrated schema produces errors
 * that look like application bugs.
 */
export async function GET(): Promise<Response> {
  const health = await pingDatabase();
  if (!health.ok) {
    // Key order matches Go's encoding/json map marshalling (keys sorted), so
    // the body is byte-identical for the diff harness.
    return statusResponse(503, { reason: health.reason, status: 'unavailable' });
  }
  return statusResponse(200, { status: 'ready' });
}
