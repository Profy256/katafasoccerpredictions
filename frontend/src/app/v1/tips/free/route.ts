import { handleFreeTips } from '@/server/api/freetips';
import { withApi, withOptions } from '@/server/http/middleware';

export const GET = withApi(handleFreeTips);
export const OPTIONS = withOptions;
