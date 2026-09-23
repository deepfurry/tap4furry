import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { Link, useParams } from '@tanstack/react-router';
import {
  getContribution,
  acceptContribution,
  rejectContribution,
  type ContributionDetail,
  type ContributionContent,
} from '@tap4furry/api-client/admin';
import {
  result,
  readOptions,
  writeOptions,
  errorMessage,
  AdminRequestError,
} from '../../lib/admin-api';
import { useDirtyGuard } from '../../lib/curation';
import { keys } from '../../lib/query-keys';
import { canEditorial } from '../../lib/capabilities';
import { useRoles } from '../../layouts/AdminShell';
import { Panel, Field, Button, Badge } from '../../components/admin/Primitives';
import { DirtyFormGuard } from '../../components/admin/DirtyFormGuard';
import { ReviewFields, ReviewSnapshot, readReview } from './ReviewFields';
export function ContributionReviewPage() {
  const { contributionId = '' } = useParams({ strict: false });
  const allowed = canEditorial(useRoles());
  const [revision, setRevision] = useState(0);
  const query = useQuery({
    queryKey: ['contributions', contributionId],
    queryFn: async () => result(await getContribution(contributionId, readOptions)),
    enabled: allowed,
    retry: false,
    refetchOnWindowFocus: false,
  });
  if (!allowed) return <p>Editorial capability is required.</p>;
  if (query.error) return <p role="alert">{errorMessage(query.error)}</p>;
  if (!query.data) return <p role="status">Loading review…</p>;
  return (
    <Review
      key={`${contributionId}:${query.data.status}:${revision}`}
      detail={query.data}
      reload={async () => {
        const response = await query.refetch();
        if (response.isSuccess) setRevision((v) => v + 1);
      }}
    />
  );
}
function Review({
  detail,
  reload,
}: {
  detail: ContributionDetail;
  reload: () => Promise<void>;
}) {
  const client = useQueryClient();
  const [dirty, setDirty] = useState(false);
  const [preview, setPreview] = useState<ContributionContent>();
  const [message, setMessage] = useState('');
  const [note, setNote] = useState('');
  const [confirmed, setConfirmed] = useState(false);
  const blocker = useDirtyGuard(dirty);
  const mutation = useMutation({
    retry: false,
    mutationFn: async (
      action: { type: 'accept'; content: ContributionContent } | { type: 'reject' },
    ) => {
      const options = await writeOptions();
      if (action.type === 'accept')
        result(
          await acceptContribution(
            detail.id,
            {
              content: action.content,
              message: message || null,
              internal_note: note || null,
            },
            options,
          ),
        );
      else
        result(
          await rejectContribution(
            detail.id,
            { message, internal_note: note || null },
            options,
          ),
        );
    },
    onSuccess: async () => {
      setDirty(false);
      await Promise.all([
        client.invalidateQueries({ queryKey: ['contributions'] }),
        client.invalidateQueries({ queryKey: keys.resources }),
      ]);
    },
  });
  const editing = detail.kind === 'update_resource';
  const pending = detail.status === 'pending';
  const blocked = !pending || detail.self_review || mutation.isPending;
  const conflict = mutation.error instanceof AdminRequestError && mutation.error.status === 409;
  const versionConflict =
    conflict &&
    mutation.error instanceof AdminRequestError &&
    mutation.error.code === 'RESOURCE_VERSION_CONFLICT';
  return (
    <>
      <DirtyFormGuard blocker={blocker} />
      <Link to="/contributions">Back to review queue</Link>
      <header className="grid gap-3">
        <h1>{detail.proposed.name}</h1>
        <div>
          <Badge>{detail.status}</Badge> · {detail.kind}
        </div>
        <p>
          Submitted by {detail.author_id} · {new Date(detail.created_at).toLocaleString()}
        </p>
        <p className="whitespace-pre-wrap">Reason: {detail.reason}</p>
      </header>
      {detail.self_review && (
        <p role="alert">You cannot review your own proposal. Another editor must decide.</p>
      )}
      {detail.conflict && pending && (
        <p role="alert">
          The target is unavailable or has changed since submission. This proposal cannot be
          accepted; ask for a fresh proposal.
        </p>
      )}
      <div className="grid gap-5 xl:grid-cols-2">
        <Panel title="Original proposal">
          <ReviewSnapshot content={detail.proposed} comparison={detail.base} />
        </Panel>
        {detail.current && (
          <Panel title="Current canonical content">
            <ReviewSnapshot content={detail.current} comparison={detail.base} />
          </Panel>
        )}
        {detail.accepted && (
          <Panel title="Accepted content">
            <ReviewSnapshot content={detail.accepted} comparison={detail.proposed} />
          </Panel>
        )}
      </div>
      {detail.base && (
        <details>
          <summary>Base at submission</summary>
          <ReviewSnapshot content={detail.base} />
        </details>
      )}
      {pending && (
        <Panel title={editing ? 'Review correction' : 'Review new draft'}>
          <form
            className="grid gap-5"
            onChange={() => {
              setDirty(true);
              setPreview(undefined);
              setConfirmed(false);
            }}
            onSubmit={(event) => {
              event.preventDefault();
              setPreview(readReview(new FormData(event.currentTarget), editing));
              setConfirmed(false);
            }}
          >
            <fieldset disabled={blocked || detail.conflict} className="grid gap-5">
              <ReviewFields value={detail.proposed} editing={editing} />
              <Button>Preview final content</Button>
            </fieldset>
          </form>
          <Field
            label="Message to author"
            hint="Required for rejection and when revising the proposal. Do not include private internal information."
          >
            <textarea
              value={message}
              rows={3}
              onChange={(e) => {
                setMessage(e.target.value);
                setDirty(true);
              }}
              disabled={blocked}
            />
          </Field>
          <Field label="Private review note" hint="Only reviewers can see this note.">
            <textarea
              value={note}
              rows={3}
              onChange={(e) => {
                setNote(e.target.value);
                setDirty(true);
              }}
              disabled={blocked}
            />
          </Field>
          {preview && (
            <section className="grid gap-4">
              <h3>Final content · changes from original marked below</h3>
              <ReviewSnapshot content={preview} comparison={detail.proposed} />
              <label className="flex gap-3">
                <input
                  type="checkbox"
                  checked={confirmed}
                  onChange={(e) => setConfirmed(e.target.checked)}
                />
                I have reviewed this final content.
              </label>
              <Button
                disabled={blocked || !confirmed || detail.conflict || versionConflict}
                onClick={() => mutation.mutate({ type: 'accept', content: preview })}
              >
                {editing ? 'Accept correction' : 'Accept as Draft'}
              </Button>
            </section>
          )}
          <div>
            <Button
              disabled={blocked || !message.trim()}
              onClick={() => {
                if (window.confirm('Reject this proposal with the message shown above?'))
                  mutation.mutate({ type: 'reject' });
              }}
            >
              Reject with reason
            </Button>
          </div>
        </Panel>
      )}
      {mutation.error && (
        <p role="alert">
          {errorMessage(mutation.error)}
          {conflict && ' Your form has been preserved. No changes were applied.'}
        </p>
      )}
      {(conflict || (detail.conflict && pending)) && (
        <Button
          onClick={() => {
            if (
              !dirty ||
              window.confirm('Discard unsaved input and reload the server record?')
            ) {
              setDirty(false);
              void reload();
            }
          }}
        >
          Reload canonical review
        </Button>
      )}
      {detail.result_resource_id && (
        <Link to="/resources/$resourceId" params={{ resourceId: detail.result_resource_id }}>
          Open resulting Resource
        </Link>
      )}
      <Panel title="Review history">
        <ol className="grid gap-4">
          {detail.history.map((event) => (
            <li key={event.event_type}>
              <strong>{event.event_type}</strong> ·{' '}
              {new Date(event.occurred_at).toLocaleString()} · {event.actor_id}
              <p className="whitespace-pre-wrap">{event.message}</p>
              {event.internal_note && (
                <p className="whitespace-pre-wrap">Private note: {event.internal_note}</p>
              )}
            </li>
          ))}
        </ol>
      </Panel>
    </>
  );
}
