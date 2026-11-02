import { handleLeagues } from '@/server/api/reference';
import { withApi, withOptions } from '@/server/http/middleware';

export const GET = withApi(handleLeagues);
export const OPTIONS = withOptions;
