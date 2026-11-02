import { handleSettledPredictions } from '@/server/api/ledger';
import { withApi, withOptions } from '@/server/http/middleware';

export const GET = withApi(handleSettledPredictions);
export const OPTIONS = withOptions;
