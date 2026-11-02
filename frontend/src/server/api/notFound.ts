import { problemResponse } from '@/server/http/render';

/**
 * The catch-all answer for a path with no registered route — port of the
 * Go mux's NotFoundHandler. Any method: Go answered OPTIONS via the CORS
 * middleware before routing, and every other method with this problem.
 */
export function routeNotFound(request: Request): Response {
  const path = new URL(request.url).pathname;
  return problemResponse(request, 404, 'not-found', 'Not found', `No route for ${request.method} ${path}`);
}
