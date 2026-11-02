import { routeNotFound } from '@/server/api/notFound';
import { withOptions } from '@/server/http/middleware';

/**
 * Any /v1 path without a more specific route gets the same problem+json the
 * Go mux produced, so an unknown endpoint is still an API response and not the
 * website's HTML 404. OPTIONS is the exception: Go's CORS middleware answered
 * preflight before routing, so unknown paths still get a 204.
 */
export const GET = routeNotFound;
export const POST = routeNotFound;
export const PUT = routeNotFound;
export const PATCH = routeNotFound;
export const DELETE = routeNotFound;
export const HEAD = routeNotFound;
export const OPTIONS = withOptions;
