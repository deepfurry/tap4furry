export function auditAdminCuration(entries) {
  const files = new Map(entries);
  const get = path => files.get(`apps/admin/src/${path}`) ?? '';
  const router = get('router.tsx');
  for (const path of ['/resources', '/resources/new', '/resources/$resourceId', '/localizations', '/tags', '/sources', '/relations', '/external-ids', '/taxonomy/categories', '/taxonomy/tags', '/account']) {
    if (!router.includes(`path: '${path}'`)) throw new Error(`Missing real Admin route: ${path}`);
  }
  if (!router.includes("redirect({ to: '/resources' })")) throw new Error('Admin root must redirect to Resources');
  if (!get('lib/capabilities.ts').includes('UX only. Backend capability checks are authoritative.')) throw new Error('Admin capability helper must remain UX-only');
  const canonical = get('lib/curation.ts');
  if (!canonical.includes('retry: false') || !canonical.includes('invalidateQueries') || !canonical.includes('enableBeforeUnload') || !canonical.includes('useBlocker')) throw new Error('Canonical mutation and dirty-form guards missing');
  for (const [path, source] of entries) {
    if (!path.startsWith('apps/admin/src/')) continue;
    if (/\bonMutate\s*:|\bsetQueryData\s*\(|\bretry\s*:\s*(?:true|[1-9])|localStorage|setInterval\s*\(|dangerouslySetInnerHTML/.test(source)) throw new Error(`Unsafe canonical UI behavior: ${path}`);
  }
  if (get('main.tsx').split('\n').length > 40 || router.split('\n').length > 200) throw new Error('Keep business UI out of Admin boot/router');
  for (const page of ['pages/resources/ResourceOverviewPage.tsx', 'pages/taxonomy/TaxonomyPage.tsx']) {
    if (!get(page).includes('<ConfirmDialog')) throw new Error('Use shared typed soft-delete confirmation');
  }
  const confirm = get('components/admin/ConfirmDialog.tsx');
  if (!confirm.includes('typed === slug') || !confirm.includes('typed !== slug')) throw new Error('Soft deletion requires typed slug');
  if (/deleteSource|Delete source|Delete Source/.test(get('pages/resources/ResourceSourcesPage.tsx'))) throw new Error('Sources must be retained');
  for (const page of ['ResourceOverviewPage', 'ResourceLocalizationsPage']) if (!get(`pages/resources/${page}.tsx`).includes('useDirtyGuard')) throw new Error('Resource form lacks dirty guard');
  if (!get('components/admin/MutationStatus.tsx').includes('Reload latest version') || !get('components/admin/MutationStatus.tsx').includes('RESOURCE_VERSION_CONFLICT')) throw new Error('Version conflict recovery must be explicit');
}
