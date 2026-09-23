import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Link } from '@tanstack/react-router';
import {
  listContributions,
  type ContributionKind,
  type ContributionStatus,
} from '@tap4furry/api-client/admin';
import { readOptions, result, errorMessage } from '../../lib/admin-api';
import { useRoles } from '../../layouts/AdminShell';
import { canEditorial } from '../../lib/capabilities';
import { Panel, Field, Badge, Button, EmptyState } from '../../components/admin/Primitives';
export function ContributionListPage() {
  const allowed = canEditorial(useRoles());
  const [page, setPage] = useState(1);
  const [status, setStatus] = useState<ContributionStatus | ''>('pending');
  const [kind, setKind] = useState<ContributionKind | ''>('');
  const query = useQuery({
    queryKey: ['contributions', { page, status, kind }],
    enabled: allowed,
    retry: false,
    queryFn: async () =>
      result(
        await listContributions(
          {
            page,
            page_size: 20,
            ...(status && { status }),
            ...(kind && { kind }),
          },
          readOptions,
        ),
      ),
  });
  if (!allowed) return <p>Editorial capability is required to review contributions.</p>;
  return (
    <>
      <h1>Contribution review</h1>
      <p>
        Review suggestions before they change canonical knowledge. Accepting a new Resource
        creates a draft.
      </p>
      <Panel title="Review queue">
        <div className="grid gap-4 sm:grid-cols-2">
          <Field label="Status">
            <select
              value={status}
              onChange={(e) => {
                setPage(1);
                setStatus(e.target.value as ContributionStatus | '');
              }}
            >
              <option value="">All</option>
              {['pending', 'accepted', 'rejected', 'withdrawn'].map((v) => (
                <option key={v}>{v}</option>
              ))}
            </select>
          </Field>
          <Field label="Kind">
            <select
              value={kind}
              onChange={(e) => {
                setPage(1);
                setKind(e.target.value as ContributionKind | '');
              }}
            >
              <option value="">All</option>
              <option value="create_resource">New Resource</option>
              <option value="update_resource">Correction</option>
            </select>
          </Field>
        </div>
        {query.error && <p role="alert">{errorMessage(query.error)}</p>}
        {query.data ? (
          <>
            {query.data.items.length ? (
              <ul className="grid gap-5">
                {query.data.items.map((v) => (
                  <li key={v.id} className="flex flex-wrap items-center justify-between gap-3">
                    <div>
                      <Link
                        to="/contributions/$contributionId"
                        params={{ contributionId: v.id }}
                      >
                        {v.name}
                      </Link>
                      <p>
                        {v.kind} · {new Date(v.created_at).toLocaleString()}
                      </p>
                    </div>
                    <Badge>{v.status}</Badge>
                  </li>
                ))}
              </ul>
            ) : (
              <EmptyState>No proposals in this view.</EmptyState>
            )}
            <nav className="flex items-center gap-4" aria-label="Review pages">
              <Button disabled={page === 1} onClick={() => setPage(page - 1)}>
                Previous
              </Button>
              <span>Page {page}</span>
              <Button disabled={!query.data.has_next} onClick={() => setPage(page + 1)}>
                Next
              </Button>
            </nav>
          </>
        ) : (
          !query.error && <p role="status">Loading proposals…</p>
        )}
      </Panel>
    </>
  );
}
