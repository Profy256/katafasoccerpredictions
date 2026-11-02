import { handleAnalysts } from '@/server/api/pro';
import { withApi, withOptions } from '@/server/http/middleware';

export const GET = withApi(handleAnalysts);
export const OPTIONS = withOptions;
