import { useEffect, useState } from 'react';
import {
  getMyContribution,
  listCategories,
  withdrawContribution,
  type ContributionDetail as Detail,
} from '@tap4furry/api-client/public';
import { checked, privateRead, contributionError } from '../../lib/contributions';
import { authenticatedRequest } from '../../lib/security';
import { ContentSnapshot } from './ContentSnapshot';
export function ContributionDetail({ id }: { id: string }) {
  const [data, setData] = useState<Detail>();
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const [categoryNames, setCategoryNames] = useState<Record<string, string>>({});
  useEffect(() => {
    let active = true;
    void listCategories(undefined, privateRead)
      .then((response) => {
        if (active && response.status === 200)
          setCategoryNames(Object.fromEntries(response.data.items.map((v) => [v.id, v.name])));
      })
      .catch(() => {
        /* History remains usable when category labels are unavailable. */
      });
    void getMyContribution(id, privateRead)
      .then((response) => {
        const value = checked(response);
        if (active) setData(value);
      })
      .catch((err) => {
        if (active) setError(contributionError(err));
      });
    return () => {
      active = false;
    };
  }, [id]);
  return (
    <div className="grid gap-6">
      <a href="/me/contributions">My proposals</a>
      {error && <p role="alert">{error}</p>}
      {data ? (
        <>
          <h2>{data.proposed.name ?? 'Resource correction'}</h2>
          <p>Status: {data.status}</p>
          <section className="grid gap-3">
            <h3>Your original proposal</h3>
            <ContentSnapshot
              content={data.proposed}
              categoryName={
                data.proposed.category_id ? categoryNames[data.proposed.category_id] : undefined
              }
            />
            <p className="whitespace-pre-wrap">Reason: {data.reason}</p>
          </section>
          {data.accepted && (
            <section className="grid gap-3">
              <h3>Accepted content</h3>
              <ContentSnapshot
                content={data.accepted}
                categoryName={categoryNames[data.accepted.category_id]}
              />
            </section>
          )}
          {data.result ? (
            <a href={`/resources/${data.result.slug}`}>View current Resource</a>
          ) : (
            data.status === 'accepted' && (
              <p>Accepted. The resulting content is not currently available in this view.</p>
            )
          )}
          <section className="grid gap-3">
            <h3>History</h3>
            <ol className="grid gap-3">
              {data.history.map((v) => (
                <li key={v.event_type}>
                  {v.event_type} · {new Date(v.occurred_at).toLocaleString()}
                  <p className="whitespace-pre-wrap">{v.message}</p>
                </li>
              ))}
            </ol>
          </section>
          {data.status === 'pending' && (
            <button
              disabled={busy}
              onClick={async () => {
                if (!window.confirm('Withdraw this proposal? It will remain in your history.'))
                  return;
                setBusy(true);
                setError('');
                try {
                  checked(await withdrawContribution(id, await authenticatedRequest()));
                  setData(checked(await getMyContribution(id, privateRead)));
                } catch (err) {
                  setError(contributionError(err));
                } finally {
                  setBusy(false);
                }
              }}
            >
              Withdraw proposal
            </button>
          )}
          {['rejected', 'withdrawn'].includes(data.status) &&
            (data.kind === 'create_resource' ? (
              <a href={`/submit?previous=${data.id}`}>Revise and submit a new proposal</a>
            ) : data.target ? (
              <a
                href={`/submit?previous=${data.id}&resource=${encodeURIComponent(data.target.slug)}`}
              >
                Revise using the current Resource
              </a>
            ) : (
              <p>The target Resource is unavailable. Your original proposal is preserved.</p>
            ))}
        </>
      ) : (
        !error && <p role="status">Loading proposal…</p>
      )}
    </div>
  );
}
