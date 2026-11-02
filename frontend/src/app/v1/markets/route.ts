import { handleMarkets } from '@/server/api/reference';
import { withApi, withOptions } from '@/server/http/middleware';

export const GET = withApi(handleMarkets);
export const OPTIONS = withOptions;
