import { handleFreeTipsHistory } from '@/server/api/freetips';
import { withApi, withOptions } from '@/server/http/middleware';

export const GET = withApi(handleFreeTipsHistory);
export const OPTIONS = withOptions;
