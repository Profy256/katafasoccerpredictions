import { handleMatchDetail } from '@/server/api/feed';
import { withApi, withOptions } from '@/server/http/middleware';

export const GET = withApi(handleMatchDetail);
export const OPTIONS = withOptions;
