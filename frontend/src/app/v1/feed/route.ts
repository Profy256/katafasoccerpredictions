import { handleFeed } from '@/server/api/feed';
import { withApi, withOptions } from '@/server/http/middleware';

export const GET = withApi(handleFeed);
export const OPTIONS = withOptions;
