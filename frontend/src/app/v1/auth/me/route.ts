import { handleMe } from '@/server/api/auth';
import { withApi, withOptions, requireUser } from '@/server/http/middleware';

export const GET = withApi(requireUser(handleMe));
export const OPTIONS = withOptions;
