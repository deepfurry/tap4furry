import type { Role } from '@tap4furry/api-client/admin';
// UX only. Backend capability checks are authoritative.
export const canAdminAccess = (roles: readonly Role[]) =>
  roles.some((role) => ['moderator', 'editor', 'admin'].includes(role));
export const canEditorial = (roles: readonly Role[]) =>
  roles.includes('editor') || roles.includes('admin');
export const canModerate = (roles: readonly Role[]) =>
  roles.includes('moderator') || roles.includes('admin');
export const canAdministrate = (roles: readonly Role[]) => roles.includes('admin');
