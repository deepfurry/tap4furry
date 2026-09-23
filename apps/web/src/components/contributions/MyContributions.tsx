import { useEffect, useState } from 'react';
import {
  listMyContributions,
  type ContributionList,
  type ContributionStatus,
} from '@tap4furry/api-client/public';
import { checked, privateRead, contributionError } from '../../lib/contributions';
import styles from '../Account.module.scss';
export function MyContributions() {
  const [page, setPage] = useState(1);
  const [status, setStatus] = useState<ContributionStatus | ''>('');
  const [data, setData] = useState<ContributionList>();
  const [error, setError] = useState('');
  useEffect(() => {
    let active = true;
    void listMyContributions({ page, page_size: 20, ...(status && { status }) }, privateRead)
      .then((response) => {
        const value = checked(response);
        if (active) {
          setData(value);
          setError('');
        }
      })
      .catch((err) => {
        if (active) setError(contributionError(err));
      });
    return () => {
      active = false;
    };
  }, [page, status]);
  return (
    <div className="grid gap-5">
      <a href="/submit">Suggest a new Resource</a>
      <label>
        Status{' '}
        <select
          className={styles.input}
          value={status}
          onChange={(e) => {
            setData(undefined);
            setPage(1);
            setStatus(e.target.value as ContributionStatus | '');
          }}
        >
          <option value="">All</option>
          {['pending', 'accepted', 'rejected', 'withdrawn'].map((v) => (
            <option key={v}>{v}</option>
          ))}
        </select>
      </label>
      {error && <p role="alert">{error}</p>}
      {data ? (
        <>
          <p>
            {data.limits.remaining_24h} submissions left in the rolling 24-hour limit ·{' '}
            {data.limits.pending_count}/{data.limits.pending_limit} pending
            {data.limits.retry_after_seconds > 0 &&
              ` · Try again after ${data.limits.retry_after_seconds} seconds`}
          </p>
          {data.items.length ? (
            <ul className="grid gap-4">
              {data.items.map((v) => (
                <li key={v.id}>
                  <a href={`/contributions/${v.id}`}>{v.name}</a>
                  <p>
                    {v.kind === 'create_resource' ? 'New Resource' : 'Correction'} · {v.status}{' '}
                    · {new Date(v.created_at).toLocaleString()}
                  </p>
                </li>
              ))}
            </ul>
          ) : (
            <p>No proposals in this view.</p>
          )}
          <nav className="flex gap-4" aria-label="Proposal pages">
            <button
              disabled={page === 1}
              onClick={() => {
                setData(undefined);
                setPage(page - 1);
              }}
            >
              Previous
            </button>
            <span>Page {page}</span>
            <button
              disabled={!data.has_next}
              onClick={() => {
                setData(undefined);
                setPage(page + 1);
              }}
            >
              Next
            </button>
          </nav>
        </>
      ) : (
        !error && <p role="status">Loading your proposals…</p>
      )}
    </div>
  );
}
