import { DirtyFormGuard } from '../../components/admin/DirtyFormGuard';
import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { useNavigate } from '@tanstack/react-router';
import {
  deleteResource,
  listCategories,
  patchResource,
  setPublication,
  type PatchResource,
  type PublicationState,
} from '@tap4furry/api-client/admin';
import { result, readOptions, writeOptions } from '../../lib/admin-api';
import { canAdministrate, canEditorial } from '../../lib/capabilities';
import { useCanonicalMutation, useDirtyGuard } from '../../lib/curation';
import { keys } from '../../lib/query-keys';
import { useRoles } from '../../layouts/AdminShell';
import { Panel, Field, Button } from '../../components/admin/Primitives';
import { ConfirmDialog } from '../../components/admin/ConfirmDialog';
import { MutationStatus, isVersionConflict } from '../../components/admin/MutationStatus';
import { useResource } from './ResourceLayout';
export function ResourceOverviewPage() {
  const { resource: r, reload } = useResource();
  const roles = useRoles();
  const navigate = useNavigate();
  const [dirty, setDirty] = useState(false);
  const blocker = useDirtyGuard(dirty);
  const categories = useQuery({
    queryKey: keys.categories,
    queryFn: async () => result(await listCategories(readOptions)),
    retry: false,
  });
  const mutation = useCanonicalMutation(
    async (patch: PatchResource) =>
      result(
        await patchResource(r.id, patch, { expected_version: r.version }, await writeOptions()),
      ),
    () => setDirty(false),
  );
  const publication = useCanonicalMutation(async (state: PublicationState) =>
    result(
      await setPublication(r.id, { state }, { expected_version: r.version }, await writeOptions()),
    ),
  );
  const deletion = useCanonicalMutation(async () => {
    result(await deleteResource(r.id, { expected_version: r.version }, await writeOptions()));
    setDirty(false);
    await navigate({ to: '/resources' });
  });
  const error = mutation.error ?? publication.error ?? deletion.error;
  const blocked = isVersionConflict(error);
  const pending = mutation.isPending || publication.isPending || deletion.isPending;
  const governed = ['restricted', 'removed'].includes(r.publication_state);
  const actions: { state: PublicationState; label: string }[] = [
    { state: 'draft', label: 'Move to Draft' },
    { state: 'pending', label: 'Move to Pending' },
    { state: 'published', label: 'Publish' },
    { state: 'restricted', label: 'Restrict' },
    { state: 'removed', label: 'Remove' },
  ];
  return (
    <>
      <DirtyFormGuard blocker={blocker} />
      <Panel title="Overview">
        <form
          className="flex flex-col gap-5"
          onChange={() => setDirty(true)}
          onSubmit={(event) => {
            event.preventDefault();
            const data = new FormData(event.currentTarget);
            mutation.mutate({
              slug: String(data.get('slug')),
              default_locale: String(data.get('locale')),
              category_id: String(data.get('category')),
              lifecycle: String(data.get('lifecycle')) as PatchResource['lifecycle'],
              content_rating: String(data.get('rating')) as PatchResource['content_rating'],
            });
          }}
        >
          <fieldset
            className="grid gap-5 sm:grid-cols-2"
            disabled={!canEditorial(roles) || pending || blocked}
          >
            <Field
              label="Slug"
              hint={r.published_at ? 'Frozen after first publication' : undefined}
            >
              <input
                name="slug"
                defaultValue={r.slug}
                readOnly={r.published_at !== null}
                required
                maxLength={80}
              />
            </Field>
            <Field label="Default locale">
              <select name="locale" defaultValue={r.default_locale}>
                {r.localizations.map((item) => (
                  <option key={item.locale}>{item.locale}</option>
                ))}
              </select>
            </Field>
            <Field label="Category">
              <select name="category" defaultValue={r.category.id}>
                <option value={r.category.id}>
                  {r.category.name}
                  {r.category.state === 'retired' ? ' — Retired (current)' : ''}
                </option>
                {categories.data?.items
                  .filter((item) => item.state === 'active' && item.id !== r.category.id)
                  .map((item) => (
                    <option key={item.id} value={item.id}>
                      {item.name}
                    </option>
                  ))}
              </select>
            </Field>
            <Field label="Lifecycle">
              <select name="lifecycle" defaultValue={r.lifecycle}>
                {['unknown', 'active', 'inactive', 'discontinued', 'delisted', 'archived'].map(
                  (v) => (
                    <option key={v}>{v}</option>
                  ),
                )}
              </select>
            </Field>
            <Field label="Content rating">
              <select name="rating" defaultValue={r.content_rating}>
                {['general', 'mature', 'explicit'].map((v) => (
                  <option key={v}>{v}</option>
                ))}
              </select>
            </Field>
            <div className="self-end">
              <Button>Save overview</Button>
            </div>
          </fieldset>
        </form>
      </Panel>
      <Panel title="Publication">
        <p>Current: {r.publication_state}</p>
        {r.published_at && <p>First published: {new Date(r.published_at).toLocaleString()}</p>}
        <div className="flex flex-wrap gap-3">
          {actions
            .filter(
              (action) =>
                action.state !== r.publication_state &&
                (!['restricted', 'removed'].includes(action.state) || canAdministrate(roles)),
            )
            .map((action) => (
              <Button
                key={action.state}
                disabled={
                  pending ||
                  blocked ||
                  dirty ||
                  !canEditorial(roles) ||
                  (governed && !canAdministrate(roles))
                }
                onClick={() => publication.mutate(action.state)}
              >
                {action.label}
              </Button>
            ))}
        </div>
        {dirty && <small>Save or reload the overview before changing publication.</small>}
        {governed && !canAdministrate(roles) && (
          <p>Only an administrator can change this publication state.</p>
        )}
      </Panel>
      {canAdministrate(roles) && (
        <Panel title="Danger Zone">
          <p>
            Soft deletion hides this Resource from Public and normal Admin browsing. Restoration is
            not available from Admin yet.
          </p>
          <ConfirmDialog
            slug={r.slug}
            disabled={pending || blocked || dirty}
            onConfirm={() => deletion.mutate()}
          />
        </Panel>
      )}
      <MutationStatus
        error={error}
        success={mutation.isSuccess}
        reload={() => {
          setDirty(false);
          void reload();
        }}
      />
    </>
  );
}
