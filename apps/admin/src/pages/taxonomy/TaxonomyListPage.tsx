import { useQuery } from '@tanstack/react-query';
import { Link, useNavigate } from '@tanstack/react-router';
import {
  createCategory,
  createTag,
  listCategories,
  listTags,
  type CreateTaxonomy,
} from '@tap4furry/api-client/admin';
import { result, readOptions, writeOptions, errorMessage } from '../../lib/admin-api';
import { useCanonicalMutation } from '../../lib/curation';
import { keys } from '../../lib/query-keys';
import { canEditorial } from '../../lib/capabilities';
import { useRoles } from '../../layouts/AdminShell';
import { Panel, Field, Button, Badge, EmptyState } from '../../components/admin/Primitives';
import { MutationStatus } from '../../components/admin/MutationStatus';
export type TaxonomyKind = 'categories' | 'tags';
export function TaxonomyListPage({ kind }: { kind: TaxonomyKind }) {
  const allowed = canEditorial(useRoles());
  const navigate = useNavigate();
  const query = useQuery({
    queryKey: keys[kind],
    queryFn: async () =>
      result(await (kind === 'categories' ? listCategories : listTags)(readOptions)),
    retry: false,
  });
  const mutation = useCanonicalMutation(async (input: CreateTaxonomy) => {
    const created = result(
      await (kind === 'categories' ? createCategory : createTag)(input, await writeOptions()),
    );
    await navigate(
      kind === 'categories'
        ? { to: '/taxonomy/categories/$categoryId', params: { categoryId: created.id } }
        : { to: '/taxonomy/tags/$tagId', params: { tagId: created.id } },
    );
  });
  return (
    <>
      <h1>{kind === 'categories' ? 'Categories' : 'Tags'}</h1>
      <Panel title="Active and retired taxonomy">
        {query.error && <p role="alert">{errorMessage(query.error)}</p>}
        {query.isPending && <p role="status">Loading taxonomy…</p>}
        {query.data?.items.length === 0 && <EmptyState>No taxonomy yet.</EmptyState>}
        <ul className="flex flex-col gap-4">
          {query.data?.items.map((item) => (
            <li key={item.id} className="flex items-center justify-between gap-3">
              {kind === 'categories' ? (
                <Link to="/taxonomy/categories/$categoryId" params={{ categoryId: item.id }}>
                  {item.name} / {item.slug}
                </Link>
              ) : (
                <Link to="/taxonomy/tags/$tagId" params={{ tagId: item.id }}>
                  {item.name} / {item.slug}
                </Link>
              )}
              <Badge>{item.state}</Badge>
            </li>
          ))}
        </ul>
      </Panel>
      <Panel title={`Create ${kind === 'categories' ? 'category' : 'tag'}`}>
        <form
          onSubmit={(event) => {
            event.preventDefault();
            const data = new FormData(event.currentTarget);
            mutation.mutate({
              slug: String(data.get('slug')),
              default_locale: String(data.get('locale')),
              localization: {
                name: String(data.get('name')),
                description: String(data.get('description')) || null,
              },
            });
          }}
        >
          <fieldset disabled={!allowed || mutation.isPending} className="grid gap-4 sm:grid-cols-2">
            <Field label="Slug (immutable after creation)">
              <input name="slug" required pattern="[a-z0-9]+(-[a-z0-9]+)*" maxLength={64} />
            </Field>
            <Field label="Default locale">
              <input name="locale" defaultValue="en" required maxLength={64} />
            </Field>
            <Field label="Name">
              <input name="name" required maxLength={80} />
            </Field>
            <Field label="Description">
              <textarea name="description" maxLength={500} />
            </Field>
            <Button>Create {kind === 'categories' ? 'category' : 'tag'}</Button>
          </fieldset>
        </form>
        <MutationStatus error={mutation.error} />
      </Panel>
    </>
  );
}
