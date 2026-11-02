import { handleSlip } from '@/server/api/slips';
import { withApi, withOptions } from '@/server/http/middleware';

export const GET = withApi(handleSlip);
export const OPTIONS = withOptions;
