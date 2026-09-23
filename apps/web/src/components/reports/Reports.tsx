import { useEffect, useRef, useState } from 'react';
import {
  getMe,
  getOwnGovernance,
  getResource,
  submitReport,
  getOwnReport,
  listOwnReports,
  withdrawReport,
  updateProfile,
  type OwnReport,
  type OwnReportList,
  type OwnReportStatus,
  type ReportInput,
  type ResourceDetail,
  type OwnGovernance,
  type Me,
} from '@tap4furry/api-client/public';
import {
  checked,
  privateRead,
  contributionError,
  useContributionDirty,
} from '../../lib/contributions';
import { authenticatedRequest } from '../../lib/security';
import styles from '../Account.module.scss';

export function ReportForm() {
  const [target, setTarget] = useState<{ resource: ResourceDetail; source?: string }>();
  const [verified, setVerified] = useState(false);
  const [reason, setReason] = useState<ReportInput['reason']>('broken_link');
  const [body, setBody] = useState('');
  const [preview, setPreview] = useState(false);
  const [confirmed, setConfirmed] = useState(false);
  const [busy, setBusy] = useState(false);
  const [saved, setSaved] = useState('');
  const [error, setError] = useState('');
  const request = useRef({ signature: '', id: '' });
  useContributionDirty(body.length > 0 && !saved);
  useEffect(() => {
    let active = true;
    const query = new URLSearchParams(window.location.search);
    void Promise.all([
      getMe(privateRead),
      getResource(query.get('slug') ?? '', undefined, privateRead),
    ])
      .then(([me, response]) => {
        const account = checked(me);
        const resource = checked(response);
        const source = query.get('source_id') ?? undefined;
        if (
          resource.id !== query.get('resource_id') ||
          (source && !resource.sources.some((s) => s.id === source))
        )
          throw new Error('target');
        if (active) {
          setVerified(account.email_verified);
          setTarget({ resource, source });
        }
      })
      .catch((e) => {
        if (active) setError(contributionError(e));
      });
    return () => {
      active = false;
    };
  }, []);
  async function submit() {
    if (!target || !confirmed || !verified) return;
    setBusy(true);
    setError('');
    try {
      const content = {
        resource_id: target.resource.id,
        ...(target.source && { source_id: target.source }),
        target_kind: target.source ? ('source' as const) : ('resource' as const),
        reason,
        body: body.trim(),
      };
      const signature = JSON.stringify(content);
      if (request.current.signature !== signature)
        request.current = { signature, id: crypto.randomUUID() };
      const response = checked(
        await submitReport(
          { ...content, request_id: request.current.id },
          await authenticatedRequest(),
        ),
      );
      setSaved(response.id);
    } catch (e) {
      setError(contributionError(e));
    } finally {
      setBusy(false);
    }
  }
  if (saved)
    return (
      <p role="status">
        Report submitted privately. <a href={`/reports/${saved}`}>View your report</a>.
      </p>
    );
  return (
    <div className="grid gap-5">
      {error && <p role="alert">{error}</p>}
      {!target ? (
        !error && <p role="status">Loading report context…</p>
      ) : (
        <>
          <p>
            Report {target.source ? 'a source on' : 'the Resource'}{' '}
            <strong>{target.resource.name}</strong>.
          </p>
          {target.source && (
            <p>
              {target.resource.sources.find((s) => s.id === target.source)?.label ??
                'Selected source'}
            </p>
          )}
          <p>
            Your report is private. Staff may share a safe outcome with you; it will not become a
            public comment.
          </p>
          {!verified && (
            <p>
              Verify your email from <a href="/account">Account</a> before submitting.
            </p>
          )}
          <label>
            Reason
            <select
              className={styles.input}
              value={reason}
              disabled={busy}
              onChange={(e) => {
                setReason(e.target.value as ReportInput['reason']);
                setPreview(false);
                setConfirmed(false);
              }}
            >
              {[
                'broken_link',
                'rights_concern',
                'malicious_link',
                'privacy',
                'content_rating',
                'spam',
                'other',
              ].map((v) => (
                <option key={v} value={v}>
                  {v.replaceAll('_', ' ')}
                </option>
              ))}
            </select>
          </label>
          <label>
            What needs attention?
            <textarea
              className={styles.input}
              rows={7}
              value={body}
              disabled={busy}
              onChange={(e) => {
                setBody(e.target.value);
                setPreview(false);
                setConfirmed(false);
              }}
            />
          </label>
          <small>
            {[...body.trim()].length}/4,000 characters. Plain text; avoid unnecessary personal
            information.
          </small>
          <button
            className={styles.button}
            disabled={busy || !verified || !body.trim() || [...body.trim()].length > 4000}
            onClick={() => setPreview(true)}
          >
            Preview report
          </button>
          {preview && (
            <section className="grid gap-4" aria-label="Report preview">
              <h2>Review before sending</h2>
              <p>{reason.replaceAll('_', ' ')}</p>
              <p className="whitespace-pre-wrap">{body}</p>
              <label>
                <input
                  type="checkbox"
                  checked={confirmed}
                  onChange={(e) => setConfirmed(e.target.checked)}
                />{' '}
                I have checked the target and report text.
              </label>
              <button
                className={styles.button}
                disabled={!confirmed || busy}
                onClick={() => {
                  void submit();
                }}
              >
                {busy ? 'Submitting…' : 'Submit private report'}
              </button>
            </section>
          )}
        </>
      )}
    </div>
  );
}

export function MyReports() {
  const [page, setPage] = useState(1),
    [status, setStatus] = useState<OwnReportStatus | ''>('');
  const [data, setData] = useState<OwnReportList>(),
    [error, setError] = useState('');
  useEffect(() => {
    let active = true;
    void listOwnReports({ page, page_size: 20, ...(status && { status }) }, privateRead)
      .then((r) => {
        const value = checked(r);
        if (active) {
          setData(value);
          setError('');
        }
      })
      .catch((e) => {
        if (active) setError(contributionError(e));
      });
    return () => {
      active = false;
    };
  }, [page, status]);
  return (
    <div className="grid gap-5">
      <label>
        Status
        <select
          className={styles.input}
          value={status}
          onChange={(e) => {
            setStatus(e.target.value as OwnReportStatus | '');
            setPage(1);
            setData(undefined);
          }}
        >
          <option value="">All</option>
          {['open', 'in_review', 'resolved', 'dismissed', 'withdrawn'].map((v) => (
            <option key={v}>{v}</option>
          ))}
        </select>
      </label>
      {error && <p role="alert">{error}</p>}
      {data ? (
        <>
          {data.items.length ? (
            <ul className="grid gap-4">
              {data.items.map((r) => (
                <li key={r.id}>
                  <a href={`/reports/${r.id}`}>
                    {r.reason.replaceAll('_', ' ')} · {new Date(r.created_at).toLocaleString()}
                  </a>
                  <p>{r.status}</p>
                </li>
              ))}
            </ul>
          ) : (
            <p>No reports in this view.</p>
          )}
          <nav className="flex gap-4" aria-label="Report pages">
            <button
              disabled={page === 1}
              onClick={() => {
                setPage(page - 1);
                setData(undefined);
              }}
            >
              Previous
            </button>
            <span>Page {page}</span>
            <button
              disabled={!data.has_next}
              onClick={() => {
                setPage(page + 1);
                setData(undefined);
              }}
            >
              Next
            </button>
          </nav>
        </>
      ) : (
        !error && <p role="status">Loading your reports…</p>
      )}
    </div>
  );
}
export function ReportDetail({ id }: { id: string }) {
  const [data, setData] = useState<OwnReport>(),
    [error, setError] = useState(''),
    [busy, setBusy] = useState(false),
    [confirmed, setConfirmed] = useState(false);
  const request = useRef('');
  useEffect(() => {
    let active = true;
    void getOwnReport(id, privateRead)
      .then((r) => {
        const value = checked(r);
        if (active) setData(value);
      })
      .catch((e) => {
        if (active) setError(contributionError(e));
      });
    return () => {
      active = false;
    };
  }, [id]);
  async function withdraw() {
    setBusy(true);
    setError('');
    try {
      request.current ||= crypto.randomUUID();
      checked(
        await withdrawReport(id, { request_id: request.current }, await authenticatedRequest()),
      );
      setData(checked(await getOwnReport(id, privateRead)));
      setConfirmed(false);
    } catch (e) {
      setError(contributionError(e));
    } finally {
      setBusy(false);
    }
  }
  return (
    <div className="grid gap-5">
      {error && <p role="alert">{error}</p>}
      {data ? (
        <>
          <p>
            {data.status} · {data.reason.replaceAll('_', ' ')}
          </p>
          <h2>Your original report</h2>
          <p className="whitespace-pre-wrap">{data.body}</p>
          {data.target ? (
            <a href={`/resources/${encodeURIComponent(data.target.slug)}`}>{data.target.name}</a>
          ) : (
            <p>The target is no longer publicly available.</p>
          )}
          <h2>Updates</h2>
          <ol className="grid gap-4">
            {data.events.map((e, i) => (
              <li key={i}>
                <p>
                  {e.event_type} · {new Date(e.occurred_at).toLocaleString()}
                </p>
                {e.safe_message && <p className="whitespace-pre-wrap">{e.safe_message}</p>}
              </li>
            ))}
          </ol>
          {['open', 'in_review'].includes(data.status) && (
            <>
              <label>
                <input
                  type="checkbox"
                  checked={confirmed}
                  onChange={(e) => setConfirmed(e.target.checked)}
                />{' '}
                Withdraw this report. Its original text and history will remain.
              </label>
              <button
                className={styles.button}
                disabled={!confirmed || busy}
                onClick={() => {
                  void withdraw();
                }}
              >
                Withdraw report
              </button>
            </>
          )}
        </>
      ) : (
        !error && <p role="status">Loading report…</p>
      )}
    </div>
  );
}

export function BusinessStatus({
  onRestricted,
  onMe,
}: {
  onRestricted: (restricted: boolean) => void;
  onMe: (me: Me) => void;
}) {
  const [data, setData] = useState<OwnGovernance>(),
    [error, setError] = useState(''),
    [busy, setBusy] = useState(false);
  useEffect(() => {
    let active = true;
    void getOwnGovernance(privateRead)
      .then((r) => {
        const value = checked(r);
        if (active) {
          setData(value);
          onRestricted(
            value.restrictions.some(
              (r) => r.scope === 'all_write' || r.scope === 'public_profile_write',
            ),
          );
        }
      })
      .catch((e) => {
        if (active) setError(contributionError(e));
      });
    return () => {
      active = false;
    };
  }, [onRestricted]);
  async function closeIndexing() {
    setBusy(true);
    try {
      onMe(
        checked(
          await updateProfile({ search_engine_indexing: false }, await authenticatedRequest()),
        ),
      );
      setError('Search engine indexing disabled.');
    } catch (e) {
      setError(contributionError(e));
    } finally {
      setBusy(false);
    }
  }
  return (
    <section className="grid gap-4">
      <h2>Business status</h2>
      {error && <p role="status">{error}</p>}
      {data ? (
        <>
          <p>
            {data.contribution_quota.remaining_24h}/{data.contribution_quota.daily_limit}{' '}
            contributions remaining in 24 hours · {data.contribution_quota.pending}/
            {data.contribution_quota.pending_limit} pending.
          </p>
          <p>
            {data.report_quota.remaining_24h}/{data.report_quota.daily_limit} reports remaining.
            Reporting and account security remain available under business restrictions.
          </p>
          {data.restrictions.length ? (
            <ul>
              {data.restrictions.map((r) => (
                <li key={r.id}>
                  <strong>{r.scope.replaceAll('_', ' ')}</strong>
                  <p>{r.user_message}</p>
                  <p>
                    {r.expires_at
                      ? `Until ${new Date(r.expires_at).toLocaleString()}`
                      : 'Until revoked'}
                  </p>
                </li>
              ))}
            </ul>
          ) : (
            <p>No active business restrictions.</p>
          )}
          <a href="/me/reports">My reports</a>
          <button
            className={styles.button}
            disabled={busy}
            onClick={() => {
              void closeIndexing();
            }}
          >
            Disable public profile indexing
          </button>
        </>
      ) : (
        !error && <p role="status">Loading business status…</p>
      )}
    </section>
  );
}
