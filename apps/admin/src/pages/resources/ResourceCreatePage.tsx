import { DirtyFormGuard } from '../../components/admin/DirtyFormGuard';
import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { useNavigate } from '@tanstack/react-router';
import {
  createResource,
  listCategories,
  type CreateResource,
  type ResourceRevision,
} from '@tap4furry/api-client/admin';
import { result, readOptions, writeOptions } from '../../lib/admin-api';
import { useCanonicalMutation, useDirtyGuard } from '../../lib/curation';
import { keys } from '../../lib/query-keys';
import { canEditorial } from '../../lib/capabilities';
import { useRoles } from '../../layouts/AdminShell';
import { Panel, Field, Button } from '../../components/admin/Primitives';
import { MutationStatus } from '../../components/admin/MutationStatus';
export function ResourceCreatePage() {
  const navigate = useNavigate();
  const allowed = canEditorial(useRoles());
  const [dirty, setDirty] = useState(false);
  const blocker = useDirtyGuard(dirty);
  const categories = useQuery({
    queryKey: keys.categories,
    queryFn: async () => result(await listCategories(readOptions)),
    retry: false,
  });
  const mutation = useCanonicalMutation(async (input: CreateResource) => {
    const created = result<ResourceRevision>(await createResource(input, await writeOptions()));
    setDirty(false);
    return created;
  });
  return (
    <>
      <DirtyFormGuard blocker={blocker} />
      <h1>Create Resource</h1>
      <Panel title="Start with a draft">
        <form
          className="flex flex-col gap-5"
          onChange={() => setDirty(true)}
          onSubmit={async (event) => {
            event.preventDefault();
            const data = new FormData(event.currentTarget);
            try {
              const created = (await mutation.mutateAsync({
                slug: String(data.get('slug')),
                default_locale: String(data.get('locale')),
                category_id: String(data.get('category')),
                content_rating: String(data.get('rating')) as CreateResource['content_rating'],
                lifecycle: String(data.get('lifecycle')) as CreateResource['lifecycle'],
                localization: {
                  name: String(data.get('name')),
                  summary: String(data.get('summary')) || null,
                  description: String(data.get('description')) || null,
                },
              })) as ResourceRevision;
              await navigate({ to: '/resources/$resourceId', params: { resourceId: created.id } });
            } catch {
              /* MutationStatus presents the safe API error. */
            }
          }}
        >
          <fieldset disabled={!allowed || mutation.isPending} className="grid gap-5 sm:grid-cols-2">
            <Field label="Slug">
              <input name="slug" required pattern="[a-z0-9]+(-[a-z0-9]+)*" maxLength={80} />
            </Field>
            <Field label="Default locale">
              <input name="locale" defaultValue="en" required maxLength={64} />
            </Field>
            <Field label="Category">
              <select name="category" required>
                <option value="">Choose an active category</option>
                {categories.data?.items
                  .filter((item) => item.state === 'active')
                  .map((item) => (
                    <option key={item.id} value={item.id}>
                      {item.name}
                    </option>
                  ))}
              </select>
            </Field>
            <Field label="Content rating">
              <select name="rating" required defaultValue="">
                <option value="">Choose a rating</option>
                {['general', 'mature', 'explicit'].map((v) => (
                  <option key={v}>{v}</option>
                ))}
              </select>
            </Field>
            <Field label="Lifecycle">
              <select name="lifecycle" defaultValue="unknown">
                {['unknown', 'active', 'inactive', 'discontinued', 'delisted', 'archived'].map(
                  (v) => (
                    <option key={v}>{v}</option>
                  ),
                )}
              </select>
            </Field>
            <Field label="Name">
              <input name="name" required maxLength={160} />
            </Field>
            <Field label="Summary">
              <textarea name="summary" maxLength={500} />
            </Field>
            <Field
              label="Description"
              hint="Markdown supported. Raw HTML and embedded images are not supported. Up to 50,000 Unicode characters."
            >
              <textarea name="description" rows={10} />
            </Field>
            <Button>Create Draft</Button>
          </fieldset>
          <MutationStatus error={mutation.error} />
        </form>
      </Panel>
    </>
  );
}
