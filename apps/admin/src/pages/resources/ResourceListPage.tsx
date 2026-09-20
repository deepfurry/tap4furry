import { useState, type FormEvent } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Link } from '@tanstack/react-router';
import {
  listResources,
  listCategories,
  type ListResourcesParams,
} from '@tap4furry/api-client/admin';
import { readOptions, result, errorMessage } from '../../lib/admin-api';
import { keys } from '../../lib/query-keys';
import { canEditorial } from '../../lib/capabilities';
import { useRoles } from '../../layouts/AdminShell';
import { Panel, Field, Button, Badge, EmptyState } from '../../components/admin/Primitives';
export function ResourceListPage() {
  const [filters, setFilters] = useState<ListResourcesParams>({ page: 1, page_size: 50 });
  const roles = useRoles();
  const categories = useQuery({
    queryKey: keys.categories,
    queryFn: async () => result(await listCategories(readOptions)),
    retry: false,
  });
  const query = useQuery({
    queryKey: [...keys.resources, 'list', filters],
    queryFn: async ({ signal }) => result(await listResources(filters, { ...readOptions, signal })),
    retry: false,
  });
  function filter(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const data = new FormData(event.currentTarget);
    setFilters({
      page: 1,
      page_size: 50,
      publication_state: (String(data.get('publication_state')) ||
        undefined) as ListResourcesParams['publication_state'],
      category_id: String(data.get('category_id')) || undefined,
      slug: String(data.get('slug')).trim() || undefined,
    });
  }
  return (
    <>
      <header className="flex flex-wrap items-center justify-between gap-4">
        <h1>Resources</h1>
        {canEditorial(roles) && <Link to="/resources/new">New Resource →</Link>}
      </header>
      <Panel title="Browse canonical resources">
        <form className="grid items-end gap-4 sm:grid-cols-2 lg:grid-cols-4" onSubmit={filter}>
          <Field label="Publication">
            <select name="publication_state">
              <option value="">All states</option>
              {['draft', 'pending', 'published', 'restricted', 'removed'].map((value) => (
                <option key={value}>{value}</option>
              ))}
            </select>
          </Field>
          <Field label="Category">
            <select name="category_id">
              <option value="">All categories</option>
              {categories.data?.items.map((item) => (
                <option value={item.id} key={item.id}>
                  {item.name} ({item.state})
                </option>
              ))}
            </select>
          </Field>
          <Field label="Exact slug">
            <input name="slug" placeholder="exact-canonical-slug" maxLength={80} />
          </Field>
          <Button>Apply filters</Button>
        </form>
      </Panel>
      {(query.error || categories.error) && (
        <p role="alert">{errorMessage(query.error ?? categories.error)}</p>
      )}
      {query.isPending ? (
        <p role="status">Loading Resources…</p>
      ) : (
        query.data && (
          <>
            <div className="overflow-x-auto">
              <table>
                <thead>
                  <tr>
                    {[
                      'Name / Slug',
                      'Category',
                      'Publication',
                      'Lifecycle',
                      'Rating',
                      'Version',
                      'Updated',
                    ].map((title) => (
                      <th key={title} scope="col">
                        {title}
                      </th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {query.data.items.map((item) => (
                    <tr key={item.id}>
                      <td>
                        <Link to="/resources/$resourceId" params={{ resourceId: item.id }}>
                          {item.name}
                        </Link>
                        <br />
                        <small>{item.slug}</small>
                      </td>
                      <td>{item.category.name}</td>
                      <td>
                        <Badge>{item.publication_state}</Badge>
                      </td>
                      <td>{item.lifecycle}</td>
                      <td>{item.content_rating}</td>
                      <td>{item.version}</td>
                      <td>{new Date(item.updated_at).toLocaleString()}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            {query.data.items.length === 0 && (
              <EmptyState>No Resources match these filters.</EmptyState>
            )}
            <div className="flex items-center gap-4">
              <Button
                disabled={filters.page === 1}
                onClick={() => setFilters({ ...filters, page: (filters.page ?? 1) - 1 })}
              >
                Previous
              </Button>
              <span>Page {filters.page}</span>
              <Button
                disabled={!query.data.has_next}
                onClick={() => setFilters({ ...filters, page: (filters.page ?? 1) + 1 })}
              >
                Next
              </Button>
            </div>
          </>
        )
      )}
    </>
  );
}
