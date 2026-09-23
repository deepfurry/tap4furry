import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Link, useParams } from '@tanstack/react-router';
import {
  listReports,
  getReport,
  getResource,
  triageReport,
  dismissReport,
  resolveReport,
  type AdminReport,
  type ListReportsParams,
  type ReportResolve,
  type ReportTriage,
} from '@tap4furry/api-client/admin';
import { result, readOptions, writeOptions, errorMessage, requireAdmin } from '../../lib/admin-api';
import { canModerate, canAdministrate, canEditorial } from '../../lib/capabilities';
import { useRoles } from '../../layouts/AdminShell';
import { useDirtyGuard } from '../../lib/curation';
import { keys } from '../../lib/query-keys';
import { DirtyFormGuard } from '../../components/admin/DirtyFormGuard';
import { Panel, Field, Button, Badge } from '../../components/admin/Primitives';
import { Pages, DecisionStatus, useGovernanceMutation, useRequestKey } from './shared';

export function ReportsPage() {
  const allowed = canModerate(useRoles());
  const [page, setPage] = useState(1),
    [status, setStatus] = useState<ListReportsParams['status']>('open'),
    [queue, setQueue] = useState<ListReportsParams['queue']>(),
    [reason, setReason] = useState<ListReportsParams['reason']>();
  const q = useQuery({
    queryKey: ['reports', { page, status, queue, reason }],
    queryFn: async () =>
      result(await listReports({ page, page_size: 20, status, queue, reason }, readOptions)),
    enabled: allowed,
    retry: false,
  });
  if (!allowed) return <p>Moderation capability is required.</p>;
  return (
    <>
      <h1>Private reports</h1>
      <Panel title="Report queue">
        <p>Review each case on its evidence. Report volume does not automatically hide content.</p>
        <div className="grid gap-4 sm:grid-cols-3">
          <Field label="Status">
            <select
              value={status ?? ''}
              onChange={(e) => {
                setStatus((e.target.value as ListReportsParams['status']) || undefined);
                setPage(1);
              }}
            >
              <option value="">All</option>
              {['open', 'in_review', 'resolved', 'dismissed', 'withdrawn'].map((v) => (
                <option key={v}>{v}</option>
              ))}
            </select>
          </Field>
          <Field label="Handling group">
            <select
              value={queue ?? ''}
              onChange={(e) => {
                setQueue((e.target.value as ListReportsParams['queue']) || undefined);
                setPage(1);
              }}
            >
              <option value="">All</option>
              <option>moderation</option>
              <option>administration</option>
            </select>
          </Field>
          <Field label="Reason">
            <select
              value={reason ?? ''}
              onChange={(e) => {
                setReason((e.target.value as ListReportsParams['reason']) || undefined);
                setPage(1);
              }}
            >
              <option value="">All</option>
              {[
                'broken_link',
                'rights_concern',
                'malicious_link',
                'privacy',
                'content_rating',
                'spam',
                'other',
              ].map((v) => (
                <option key={v}>{v}</option>
              ))}
            </select>
          </Field>
        </div>
        {q.error && <p role="alert">{errorMessage(q.error)}</p>}
        {q.data ? (
          <>
            {q.data.items.length ? (
              <ul className="grid gap-4">
                {q.data.items.map((r) => (
                  <li key={r.id}>
                    <Link to="/reports/$reportId" params={{ reportId: r.id }}>
                      {r.reason.replaceAll('_', ' ')} · {new Date(r.created_at).toLocaleString()}
                    </Link>
                    <p>
                      <Badge>{r.status}</Badge> {r.queue} {r.priority > 0 ? '· High priority' : ''}
                    </p>
                  </li>
                ))}
              </ul>
            ) : (
              <p>No reports in this view.</p>
            )}
            <Pages page={page} hasNext={q.data.has_next} onPage={setPage} />
          </>
        ) : (
          !q.error && <p role="status">Loading reports…</p>
        )}
      </Panel>
    </>
  );
}
export function ReportReviewPage() {
  const { reportId = '' } = useParams({ strict: false });
  const allowed = canModerate(useRoles());
  const q = useQuery({
    queryKey: ['reports', reportId],
    queryFn: async () => result(await getReport(reportId, readOptions)),
    enabled: allowed,
    retry: false,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  });
  if (!allowed) return <p>Moderation capability is required.</p>;
  if (q.error && !q.data) return <p role="alert">{errorMessage(q.error)}</p>;
  if (!q.data) return <p role="status">Loading report…</p>;
  return (
    <ReportReview
      detail={q.data}
      reload={async () => {
        await q.refetch();
      }}
    />
  );
}
function ReportReview({ detail: r, reload }: { detail: AdminReport; reload: () => Promise<void> }) {
  const roles = useRoles();
  const admin = canAdministrate(roles),
    editor = canEditorial(roles);
  const me = useQuery({ queryKey: keys.me, queryFn: requireAdmin, retry: false });
  const resource = useQuery({
    queryKey: keys.resource(r.resource_id),
    queryFn: async () => result(await getResource(r.resource_id, readOptions)),
    retry: false,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  });
  const [action, setAction] = useState('receive'),
    [mode, setMode] = useState<ReportResolve['mode']>('no_change'),
    [state, setState] = useState(''),
    [message, setMessage] = useState(''),
    [note, setNote] = useState(''),
    [reason, setReason] = useState(''),
    [audit, setAudit] = useState(''),
    [duplicate, setDuplicate] = useState('');
  const [dirty, setDirty] = useState(false),
    [confirmed, setConfirmed] = useState(false);
  const blocker = useDirtyGuard(dirty);
  const requestKey = useRequestKey();
  const terminal = !['open', 'in_review'].includes(r.status);
  const self = me.data?.id === r.reporter_id;
  const sensitive =
    r.queue === 'administration' ||
    ['rights_concern', 'privacy', 'malicious_link'].includes(r.reason);
  const mutation = useGovernanceMutation(
    async () => {
      const common = {
        expected_report_version: r.version,
        ...(note.trim() && { internal_note: note.trim() }),
      };
      const options = await writeOptions();
      if (['receive', 'escalate', 'note'].includes(action)) {
        const payload = { ...common, action: action as ReportTriage['action'] };
        return result(
          await triageReport(
            r.id,
            { ...payload, request_id: requestKey([r.id, payload]) },
            options,
          ),
        );
      }
      if (action === 'dismiss') {
        const payload = {
          ...common,
          safe_message: message.trim(),
          ...(duplicate && { duplicate_of: duplicate }),
        };
        return result(
          await dismissReport(
            r.id,
            { ...payload, request_id: requestKey([r.id, 'dismiss', payload]) },
            options,
          ),
        );
      }
      const payload: Omit<ReportResolve, 'request_id'> = {
        ...common,
        safe_message: message.trim(),
        mode,
      };
      if (mode === 'link_audit') payload.audit_id = audit;
      if (!['no_change', 'link_audit'].includes(mode)) {
        payload.expected_resource_version = resource.data?.version;
        payload.reason = reason.trim();
        if (mode === 'publication')
          payload.publication_state = state as ReportResolve['publication_state'];
        if (mode === 'source_availability')
          payload.availability_state = state as ReportResolve['availability_state'];
        if (mode === 'source_rights')
          payload.rights_status = state as ReportResolve['rights_status'];
        if (mode === 'distribution') payload.policy = state as ReportResolve['policy'];
      }
      return result(
        await resolveReport(r.id, { ...payload, request_id: requestKey([r.id, payload]) }, options),
      );
    },
    () => {
      setDirty(false);
      setConfirmed(false);
      setNote('');
    },
  );
  const options =
    mode === 'publication'
      ? ['draft', 'pending', 'published', 'restricted', 'removed']
      : mode === 'source_availability'
        ? ['active', 'unavailable', 'broken', 'restricted', 'removed']
        : mode === 'source_rights'
          ? [
              'unknown',
              'creator_provided',
              'confirmed',
              'rights_review',
              'disputed',
              'removed_by_request',
            ]
          : ['normal', 'excluded'];
  const composite = action === 'resolve' && !['no_change', 'link_audit'].includes(mode);
  const disabled = mutation.isPending || terminal || self;
  return (
    <>
      <DirtyFormGuard blocker={blocker} />
      <h1>Report review</h1>
      <Panel title="Original report">
        <p>
          <Badge>{r.status}</Badge> · {r.reason} · Version {r.version}
        </p>
        <p className="whitespace-pre-wrap">{r.body}</p>
        <Link to="/resources/$resourceId" params={{ resourceId: r.resource_id }}>
          Inspect Resource
        </Link>
        {r.source_id && <p>Source: {r.source_id}</p>}
        {admin && (
          <Link to="/governance/users/$userId" params={{ userId: r.reporter_id }}>
            View reporter business status
          </Link>
        )}
        <small>
          The reporter is not the owner of this Resource. Do not infer responsibility from an
          association.
        </small>
      </Panel>
      <Panel title="Processing history">
        {r.events.map((e) => (
          <article key={e.id}>
            <p>
              {e.event_type} · {new Date(e.occurred_at).toLocaleString()}
            </p>
            {e.safe_message && <p>Reporter message: {e.safe_message}</p>}
            {e.internal_note && <p className="whitespace-pre-wrap">Internal: {e.internal_note}</p>}
          </article>
        ))}
      </Panel>
      {!terminal && (
        <Panel title="Handle report">
          {self && <p>You cannot decide your own report.</p>}
          {sensitive && !admin && (
            <p>
              This case requires Administration for a decision. You can receive, note or escalate
              it.
            </p>
          )}
          <form
            className="grid gap-5"
            onChange={(e) => {
              setDirty(true);
              if (!(e.target instanceof HTMLInputElement && e.target.name === 'confirmation'))
                setConfirmed(false);
            }}
            onSubmit={(e) => {
              e.preventDefault();
              if (confirmed) mutation.mutate();
            }}
          >
            <fieldset disabled={disabled} className="grid gap-5">
              <Field label="Action">
                <select value={action} onChange={(e) => setAction(e.target.value)}>
                  <option value="receive">Receive</option>
                  <option value="escalate">Escalate to Administration</option>
                  <option value="note">Add internal observation</option>
                  {(!sensitive || admin) && (
                    <>
                      <option value="resolve">Resolve</option>
                      <option value="dismiss">Dismiss</option>
                    </>
                  )}
                </select>
              </Field>
              {action === 'resolve' && (
                <Field label="Resolution basis">
                  <select
                    value={mode}
                    onChange={(e) => {
                      setMode(e.target.value as ReportResolve['mode']);
                      setState('');
                    }}
                  >
                    <option value="no_change">No change needed; explain why</option>
                    <option value="link_audit">Link an existing change audit</option>
                    {editor && r.source_id && (
                      <option value="source_availability">Update source availability</option>
                    )}
                    {admin && (
                      <>
                        <option value="publication">Change Resource publication</option>
                        {r.source_id && <option value="source_rights">Change source rights</option>}
                        <option value="distribution">Set recommendation policy</option>
                      </>
                    )}
                  </select>
                </Field>
              )}
              {action === 'resolve' && mode === 'link_audit' && (
                <Field label="Existing audit ID">
                  <input value={audit} onChange={(e) => setAudit(e.target.value)} required />
                </Field>
              )}
              {composite && (
                <>
                  <p>
                    Resource baseline: version {resource.data?.version ?? 'unavailable'} ·{' '}
                    {resource.data?.publication_state}. A stale change will roll back the entire
                    decision.
                  </p>
                  <Field label="New state">
                    <select required value={state} onChange={(e) => setState(e.target.value)}>
                      <option value="">Choose explicitly</option>
                      {options.map((v) => (
                        <option key={v}>{v}</option>
                      ))}
                    </select>
                  </Field>
                  <Field label="Governance reason">
                    <textarea
                      required
                      maxLength={1000}
                      value={reason}
                      onChange={(e) => setReason(e.target.value)}
                    />
                  </Field>
                </>
              )}
              {action === 'dismiss' && (
                <Field label="Duplicate report ID (optional, internal)">
                  <input value={duplicate} onChange={(e) => setDuplicate(e.target.value)} />
                </Field>
              )}
              {['resolve', 'dismiss'].includes(action) && (
                <Field label="Message the reporter will see">
                  <textarea
                    required
                    maxLength={1000}
                    value={message}
                    onChange={(e) => setMessage(e.target.value)}
                  />
                </Field>
              )}
              <Field label="Internal note (never sent to the reporter)">
                <textarea
                  required={action === 'note'}
                  maxLength={2000}
                  value={note}
                  onChange={(e) => setNote(e.target.value)}
                />
              </Field>
              {['resolve', 'dismiss'].includes(action) && (
                <section className="grid gap-2" aria-label="Reporter message preview">
                  <h3>Reporter message preview</h3>
                  <p className="whitespace-pre-wrap">{message || 'Enter a safe outcome above.'}</p>
                </section>
              )}
              <label>
                <input
                  name="confirmation"
                  type="checkbox"
                  checked={confirmed}
                  onChange={(e) => setConfirmed(e.target.checked)}
                />{' '}
                I reviewed the action and the author-visible message.
              </label>
              <Button disabled={!confirmed || (composite && !resource.data)}>Apply decision</Button>
            </fieldset>
          </form>
        </Panel>
      )}
      <DecisionStatus
        error={mutation.error}
        success={mutation.isSuccess}
        reload={() => {
          setConfirmed(false);
          void Promise.all([reload(), resource.refetch()]);
        }}
      />
    </>
  );
}
