import { createContext, useContext } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Link, Outlet, useParams } from '@tanstack/react-router';
import { getResource, type ResourceDetail } from '@tap4furry/api-client/admin';
import { errorMessage, readOptions, result } from '../../lib/admin-api';
import { keys } from '../../lib/query-keys';
import { Badge } from '../../components/admin/Primitives';
type ResourceContextValue = { resource: ResourceDetail; reload: () => Promise<unknown> };
const ResourceContext = createContext<ResourceContextValue | null>(null);
export function useResource() {
  const context = useContext(ResourceContext);
  if (!context) throw new Error('Resource layout required');
  return context;
}
export function ResourceLayout() {
  const { resourceId } = useParams({ strict: false });
  const query = useQuery({
    queryKey: keys.resource(resourceId!),
    queryFn: async ({ signal }) =>
      result<ResourceDetail>(await getResource(resourceId!, { ...readOptions, signal })),
    retry: false,
    staleTime: Infinity,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  });
  if (query.error) return <p role="alert">{errorMessage(query.error)}</p>;
  if (!query.data) return <p role="status">Loading Resource…</p>;
  const resource = query.data;
  const publicOrigin = import.meta.env.DEV ? 'http://localhost:4321' : 'https://tap4furry.com';
  return (
    <>
      <header className="flex flex-col gap-3">
        <p>Resource / {resource.slug}</p>
        <h1>
          {resource.localizations.find((item) => item.locale === resource.default_locale)?.name}
        </h1>
        <div className="flex flex-wrap gap-2">
          <Badge>{resource.publication_state}</Badge>
          <Badge>{resource.lifecycle}</Badge>
          <Badge>{resource.content_rating}</Badge>
          <Badge>Version {resource.version}</Badge>
        </div>
        {resource.publication_state === 'published' && (
          <a href={`${publicOrigin}/resources/${resource.slug}`} target="_blank" rel="noreferrer">
            View Public Page ↗
          </a>
        )}
      </header>
      <nav aria-label="Resource sections" className="flex flex-wrap gap-4">
        <Link
          to="/resources/$resourceId"
          params={{ resourceId: resource.id }}
          activeOptions={{ exact: true }}
        >
          Overview
        </Link>
        <Link to="/resources/$resourceId/localizations" params={{ resourceId: resource.id }}>
          Localizations
        </Link>
        <Link to="/resources/$resourceId/tags" params={{ resourceId: resource.id }}>
          Tags
        </Link>
        <Link to="/resources/$resourceId/sources" params={{ resourceId: resource.id }}>
          Sources
        </Link>
        <Link to="/resources/$resourceId/relations" params={{ resourceId: resource.id }}>
          Relations
        </Link>
        <Link to="/resources/$resourceId/external-ids" params={{ resourceId: resource.id }}>
          External IDs
        </Link>
      </nav>
      <ResourceContext.Provider
        key={`${resource.id}:${resource.version}:${query.dataUpdatedAt}`}
        value={{ resource, reload: query.refetch }}
      >
        <Outlet />
      </ResourceContext.Provider>
    </>
  );
}
