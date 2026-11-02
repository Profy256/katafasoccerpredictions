import { handleAnalystLeaderboard } from '@/server/api/records';
import { withApi, withOptions } from '@/server/http/middleware';

export const GET = withApi(handleAnalystLeaderboard);
export const OPTIONS = withOptions;
