import { handleAnalystRecord } from '@/server/api/records';
import { withApi, withOptions } from '@/server/http/middleware';

export const GET = withApi(handleAnalystRecord);
export const OPTIONS = withOptions;
