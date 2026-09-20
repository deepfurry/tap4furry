import type { ExternalID } from '@tap4furry/api-client/admin';
export function externalIDsError(items: readonly ExternalID[]): string | null {
  const seen = new Set<string>();
  for (const item of items) {
    if (!item.namespace.trim() || !item.external_id.trim())
      return 'Every row needs a namespace and external ID.';
    const key = JSON.stringify([item.namespace, item.external_id.trim()]);
    if (seen.has(key)) return 'Remove duplicate external ID rows.';
    seen.add(key);
  }
  return null;
}
