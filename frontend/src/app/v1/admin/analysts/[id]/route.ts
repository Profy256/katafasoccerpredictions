import { handleAdminDeactivateAnalyst } from '@/server/api/admin';
import { withApi, withOptions, requireAdmin } from '@/server/http/middleware';

export const DELETE = withApi(requireAdmin(handleAdminDeactivateAnalyst));
export const OPTIONS = withOptions;