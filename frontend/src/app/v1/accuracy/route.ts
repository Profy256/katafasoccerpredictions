import { handleAccuracy } from '@/server/api/accuracy';
import { withApi, withOptions } from '@/server/http/middleware';

export const GET = withApi(handleAccuracy);
export const OPTIONS = withOptions;
