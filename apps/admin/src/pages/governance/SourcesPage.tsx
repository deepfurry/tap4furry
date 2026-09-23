import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Link } from '@tanstack/react-router';
import {
  listSourceHealth,
  recordSourceCheck,
  type SourceHealth,
  type SourceCheckInput,
  type ListSourceHealthParams,
} from '@tap4furry/api-client/admin';
import { result, readOptions, writeOptions, errorMessage } from '../../lib/admin-api';
import { canEditorial } from '../../lib/capabilities';
import { useRoles } from '../../layouts/AdminShell';
import { useDirtyGuard } from '../../lib/curation';
import { DirtyFormGuard } from '../../components/admin/DirtyFormGuard';
import { Panel, Field, Button } from '../../components/admin/Primitives';
import { Pages, DecisionStatus, useGovernanceMutation, useRequestKey } from './shared';
export function SourcesPage() {
  const [page, setPage] = useState(1),
    [availability, setAvailability] = useState<ListSourceHealthParams['availability_state']>(),
    [broken, setBroken] = useState(false),
    [selected, setSelected] = useState<SourceHealth>();
  const q = useQuery({
    queryKey: ['source-health', { page, availability, broken }],
    queryFn: async () =>
      result(
        await listSourceHealth(
          {
            page,
            page_size: 20,
            availability_state: availability,
            ...(broken && { open_broken_report: true }),
          },
          readOptions,
        ),
      ),
    retry: false,
    refetchOnWindowFocus: false,
  });
  return (
    <>
      <h1>Manual source checks</h1>
      <p>
        Record an observation about the current URL. Tap4Furry does not fetch or certify the
        destination.
      </p>
      {selected ? (
        <SourceObservation
          key={selected.source_id}
          source={selected}
          update={setSelected}
          close={() => setSelected(undefined)}
        />
      ) : (
        <Panel title="Source health">
          <div className="flex flex-wrap gap-4">
            <Field label="Availability">
              <select
                value={availability ?? ''}
                onChange={(e) => {
                  setAvailability(
                    (e.target.value as ListSourceHealthParams['availability_state']) || undefined,
                  );
                  setPage(1);
                }}
              >
                <option value="">All</option>
                {['active', 'unavailable', 'broken', 'restricted', 'removed'].map((v) => (
                  <option key={v}>{v}</option>
                ))}
              </select>
            </Field>
            <label>
              <input
                type="checkbox"
                checked={broken}
                onChange={(e) => {
                  setBroken(e.target.checked);
                  setPage(1);
                }}
              />{' '}
              Has an open broken-link report
            </label>
          </div>
          {q.error && <p role="alert">{errorMessage(q.error)}</p>}
          {q.data ? (
            <>
              {q.data.items.length ? (
                <ul className="grid gap-5">
                  {q.data.items.map((s) => (
                    <li key={s.source_id}>
                      <p>
                        {s.resource_slug} · {s.availability_state} · {s.rights_status}
                      </p>
                      <a href={s.url} target="_blank" rel="noreferrer noopener">
                        {s.url}
                      </a>
                      <p>
                        {s.last_check
                          ? `${s.last_check.outcome} · ${new Date(s.last_check.observed_at).toLocaleString()}${s.last_check.current ? '' : ' · URL changed since this check'}`
                          : 'No manual check recorded'}
                      </p>
                      <Button onClick={() => setSelected(s)}>Record observation</Button>
                    </li>
                  ))}
                </ul>
              ) : (
                <p>No sources in this view.</p>
              )}
              <Pages page={page} hasNext={q.data.has_next} onPage={setPage} />
            </>
          ) : (
            !q.error && <p role="status">Loading sources…</p>
          )}
        </Panel>
      )}
    </>
  );
}
function SourceObservation({
  source: s,
  update,
  close,
}: {
  source: SourceHealth;
  update: (s: SourceHealth) => void;
  close: () => void;
}) {
  const editor = canEditorial(useRoles());
  const [outcome, setOutcome] = useState<SourceCheckInput['outcome']>('uncertain'),
    [note, setNote] = useState(''),
    [observed, setObserved] = useState(new Date().toISOString().slice(0, 16)),
    [availability, setAvailability] = useState<SourceCheckInput['availability_state']>();
  const [reloadError, setReloadError] = useState<unknown>();
  const [dirty, setDirty] = useState(false),
    [confirmed, setConfirmed] = useState(false);
  const blocker = useDirtyGuard(dirty),
    request = useRequestKey();
  async function reload() {
    const value = result(
      await listSourceHealth(
        { resource_id: s.resource_id, source_id: s.source_id, page_size: 1 },
        readOptions,
      ),
    );
    const current = value.items.find((v) => v.source_id === s.source_id);
    if (!current) throw new Error('Source unavailable');
    update(current);
  }
  const mutation = useGovernanceMutation(
    async () => {
      const payload = {
        expected_version: s.resource_version,
        outcome,
        note: note.trim(),
        observed_at: new Date(observed + 'Z').toISOString(),
        ...(availability && { availability_state: availability }),
      };
      return result(
        await recordSourceCheck(
          s.resource_id,
          s.source_id,
          { ...payload, request_id: request([s.source_id, payload]) },
          await writeOptions(),
        ),
      );
    },
    async () => {
      setDirty(false);
      setConfirmed(false);
      setNote('');
      await reload();
    },
  );
  return (
    <>
      <DirtyFormGuard blocker={blocker} />
      <Panel title="Record a check">
        <p>
          {s.resource_slug} · Resource version {s.resource_version}
        </p>
        <a href={s.url} target="_blank" rel="noreferrer noopener">
          {s.url}
        </a>
        <Link to="/resources/$resourceId" params={{ resourceId: s.resource_id }}>
          Inspect Resource
        </Link>
        <form
          className="grid gap-4"
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
          <fieldset className="grid gap-4" disabled={mutation.isPending}>
            <Field label="Observed outcome">
              <select
                value={outcome}
                onChange={(e) => setOutcome(e.target.value as SourceCheckInput['outcome'])}
              >
                <option>reachable</option>
                <option>unreachable</option>
                <option>uncertain</option>
              </select>
            </Field>
            <Field label="Observed at (UTC)">
              <input
                type="datetime-local"
                required
                value={observed}
                onChange={(e) => setObserved(e.target.value)}
              />
            </Field>
            <Field label="Observation only; do not copy reporter identities or report text">
              <textarea
                required
                maxLength={2000}
                value={note}
                onChange={(e) => setNote(e.target.value)}
              />
            </Field>
            {editor ? (
              <Field label="Optional explicit availability update">
                <select
                  value={availability ?? ''}
                  onChange={(e) =>
                    setAvailability(
                      (e.target.value as SourceCheckInput['availability_state']) || undefined,
                    )
                  }
                >
                  <option value="">Record observation only</option>
                  {['active', 'unavailable', 'broken', 'restricted', 'removed'].map((v) => (
                    <option key={v}>{v}</option>
                  ))}
                </select>
              </Field>
            ) : (
              <p>Your role can record observations. It cannot change source availability.</p>
            )}
            <p>
              {availability
                ? `Record this observation and set availability to ${availability}. Rights remain ${s.rights_status}.`
                : 'Record this observation without changing the Resource version.'}
            </p>
            <label>
              <input
                name="confirmation"
                type="checkbox"
                checked={confirmed}
                onChange={(e) => setConfirmed(e.target.checked)}
              />{' '}
              I checked the current URL and reviewed the observation.
            </label>
            <Button disabled={!confirmed}>Save observation</Button>
          </fieldset>
        </form>
        <DecisionStatus
          error={reloadError ?? mutation.error}
          success={mutation.isSuccess}
          reload={() => {
            setConfirmed(false);
            setReloadError(undefined);
            void reload().catch(setReloadError);
          }}
        />
        <Button type="button" onClick={close} disabled={mutation.isPending}>
          {dirty ? 'Discard observation and return' : 'Back to source list'}
        </Button>
      </Panel>
    </>
  );
}
