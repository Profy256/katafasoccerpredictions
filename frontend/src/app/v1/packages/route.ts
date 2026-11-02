import { handlePackages } from '@/server/api/pro';
import { withApi, withOptions } from '@/server/http/middleware';

export const GET = withApi(handlePackages);
export const OPTIONS = withOptions;
