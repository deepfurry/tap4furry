import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Link } from '@tanstack/react-router';
import { listAudit, type ListAuditParams, type AuditEntry } from '@tap4furry/api-client/admin';
import { result, readOptions, errorMessage } from '../../lib/admin-api';
import { canAdministrate } from '../../lib/capabilities';
import { useRoles } from '../../layouts/AdminShell';
import { Panel, Field, Button } from '../../components/admin/Primitives';
import { Pages } from './shared';
export function AuditPage() {
  const allowed = canAdministrate(useRoles());
  const [page, setPage] = useState(1),
    [filters, setFilters] = useState<ListAuditParams>({});
  const q = useQuery({
    queryKey: ['audit', { page, ...filters }],
    queryFn: async () => result(await listAudit({ ...filters, page, page_size: 20 }, readOptions)),
    enabled: allowed,
    retry: false,
  });
  if (!allowed) return <p>Administration capability is required to inspect business audits.</p>;
  return (
    <>
      <h1>Business audit</h1>
      <p>
        Canonical and governance changes from this stage onward. Authentication security events and
        contribution originals remain in their own histories.
      </p>
      <Panel title="Audit timeline">
        <form
          className="grid gap-4 sm:grid-cols-3"
          onSubmit={(e) => {
            e.preventDefault();
            const values = new FormData(e.currentTarget);
            setFilters({
              resource_id: String(values.get('resource_id')) || undefined,
              user_id: String(values.get('user_id')) || undefined,
              operation:
                (String(values.get('operation')) as ListAuditParams['operation']) || undefined,
            });
            setPage(1);
          }}
        >
          <Field label="Exact Resource ID">
            <input name="resource_id" pattern="[0-9a-fA-F-]{36}" />
          </Field>
          <Field label="Exact User ID">
            <input name="user_id" pattern="[0-9a-fA-F-]{36}" />
          </Field>
          <Field label="Operation">
            <select name="operation">
              <option value="">All</option>
              {[
                'create',
                'core',
                'localization',
                'tags',
                'source',
                'source_rights',
                'relation',
                'external_ids',
                'publication',
                'soft_delete',
                'taxonomy',
                'distribution',
                'trust',
                'restrict',
                'revoke_restriction',
              ].map((v) => (
                <option key={v}>{v}</option>
              ))}
            </select>
          </Field>
          <Button>Apply filters</Button>
        </form>
        {q.error && <p role="alert">{errorMessage(q.error)}</p>}
        {q.data ? (
          <>
            {q.data.items.length ? (
              <ol className="grid gap-5">
                {q.data.items.map((a) => (
                  <li key={a.id}>
                    <AuditRecord audit={a} />
                  </li>
                ))}
              </ol>
            ) : (
              <p>No changes match these filters.</p>
            )}
            <Pages page={page} hasNext={q.data.has_next} onPage={setPage} />
          </>
        ) : (
          !q.error && <p role="status">Loading audit…</p>
        )}
      </Panel>
    </>
  );
}
function AuditRecord({ audit: a }: { audit: AuditEntry }) {
  const changes = [
    ['Publication', a.before_publication, a.after_publication],
    ['Source rights', a.before_rights, a.after_rights],
    ['Distribution', a.before_distribution, a.after_distribution],
    ['Trust', a.before_trust, a.after_trust],
    ['Availability', a.before_availability, a.after_availability],
    ['Taxonomy state', a.before_taxonomy_state, a.after_taxonomy_state],
  ];
  return (
    <details>
      <summary>
        {a.operation.replaceAll('_', ' ')} · {new Date(a.occurred_at).toLocaleString()}
      </summary>
      <div className="grid gap-3 py-3">
        <p>Audit ID: {a.id}</p>
        <p>Actor: {a.actor_id}</p>
        {a.resource_id && (
          <Link to="/resources/$resourceId" params={{ resourceId: a.resource_id }}>
            Resource {a.resource_id}
          </Link>
        )}
        {a.user_id && (
          <Link to="/governance/users/$userId" params={{ userId: a.user_id }}>
            User {a.user_id}
          </Link>
        )}
        {a.category_id && <p>Category {a.category_id}</p>}
        {a.tag_id && <p>Tag {a.tag_id}</p>}
        {a.source_id && <p>Source {a.source_id}</p>}
        <p>Changed groups: {a.fields.join(', ')}</p>
        {a.after_version !== undefined && (
          <p>
            Revision {a.before_version} → {a.after_version}
          </p>
        )}
        {changes
          .filter(([, before, after]) => before !== undefined || after !== undefined)
          .map(([label, before, after]) => (
            <p key={label}>
              {label}: {before ?? 'none'} → {after ?? 'none'}
            </p>
          ))}
        {a.reason && <p>Reason: {a.reason}</p>}
        {a.contribution_id && (
          <Link to="/contributions/$contributionId" params={{ contributionId: a.contribution_id }}>
            Contribution history
          </Link>
        )}
      </div>
    </details>
  );
}
