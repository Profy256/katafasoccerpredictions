import { handleSlips } from '@/server/api/slips';
import { withApi, withOptions } from '@/server/http/middleware';

export const GET = withApi(handleSlips);
export const OPTIONS = withOptions;
