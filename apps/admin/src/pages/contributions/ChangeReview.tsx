import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Link } from '@tanstack/react-router';
import {
  acceptContribution,
  rejectContribution,
  listTags,
  getResource,
  type ContributionChange,
  type ContributionDetail,
} from '@tap4furry/api-client/admin';
import { result, readOptions, writeOptions, errorMessage } from '../../lib/admin-api';
import { useDirtyGuard } from '../../lib/curation';
import { keys } from '../../lib/query-keys';
import { Panel, Field, Button, Badge } from '../../components/admin/Primitives';
import { DirtyFormGuard } from '../../components/admin/DirtyFormGuard';

const sourceTypes = [
  'unknown',
  'official',
  'store',
  'archive',
  'mirror',
  'community',
  'external',
] as const;
export function ChangeReview({
  detail,
  reload,
}: {
  detail: ContributionDetail;
  reload: () => Promise<void>;
}) {
  const client = useQueryClient();
  const [dirty, setDirty] = useState(false);
  const [preview, setPreview] = useState<ContributionChange>();
  const [message, setMessage] = useState('');
  const [note, setNote] = useState('');
  const [confirmed, setConfirmed] = useState(false);
  const [editing, setEditing] = useState(false);
  const [formError, setFormError] = useState('');
  const blocker = useDirtyGuard(dirty);
  const tags = useQuery({
    queryKey: ['contribution-tag-options'],
    queryFn: async () => result(await listTags(readOptions)),
    enabled: detail.kind === 'add_tag',
    retry: false,
  });
  const mutation = useMutation({
    retry: false,
    mutationFn: async (action: 'accept' | 'reject') => {
      const options = await writeOptions();
      if (action === 'accept')
        result(
          await acceptContribution(
            detail.id,
            {
              change: preview,
              message: message || null,
              internal_note: note || null,
            },
            options,
          ),
        );
      else
        result(
          await rejectContribution(detail.id, { message, internal_note: note || null }, options),
        );
    },
    onSuccess: async () => {
      setDirty(false);
      await Promise.all([
        client.invalidateQueries({ queryKey: ['contributions'] }),
        client.invalidateQueries({ queryKey: keys.resources }),
      ]);
      await reload();
    },
  });
  const proposed = detail.proposed_change!;
  const tagNames = Object.fromEntries(tags.data?.items.map((t) => [t.id, t.name]) ?? []);
  const blocked = detail.status !== 'pending' || detail.self_review || mutation.isPending;
  return (
    <div className="grid gap-5">
      <DirtyFormGuard blocker={blocker} />
      <Link to="/contributions">Back to review queue</Link>
      <header>
        <h1>{detail.kind.replaceAll('_', ' ')}</h1>
        <Badge>{detail.status}</Badge>
        <p className="whitespace-pre-wrap">Reason: {detail.reason}</p>
        {detail.target_resource_id && (
          <ResourceReference id={detail.target_resource_id} label="Target" />
        )}
        {proposed.relation && (
          <ResourceReference id={proposed.relation.other_resource_id} label="Other endpoint" />
        )}
      </header>
      {detail.self_review && <p role="alert">Another editor must review your proposal.</p>}
      {detail.conflict && detail.status === 'pending' && (
        <p role="alert">
          The target, version or selected item has changed. Preserve the original and request a
          fresh proposal when its baseline is stale.
        </p>
      )}
      <div className="grid gap-5 xl:grid-cols-2">
        {detail.base_change && (
          <Panel title="Submission baseline">
            {detail.kind === 'add_relation' && (
              <p>
                The endpoint context is fixed; the proposed relation did not exist at submission.
              </p>
            )}
            {detail.kind === 'add_source' && (
              <p>
                This proposal adds a new Source. Inspect the target Resource for its current
                sources.
              </p>
            )}
            <ChangeReviewSnapshot value={detail.base_change} tagNames={tagNames} />
          </Panel>
        )}
        <Panel title="Original proposal">
          <ChangeReviewSnapshot
            value={proposed}
            comparison={detail.base_change}
            tagNames={tagNames}
          />
        </Panel>
        {detail.current_change && (
          <Panel title="Current context">
            <ChangeReviewSnapshot
              value={detail.current_change}
              comparison={detail.base_change}
              tagNames={tagNames}
            />
          </Panel>
        )}
        {detail.accepted_change && (
          <Panel title="Accepted content">
            <ChangeReviewSnapshot
              value={detail.accepted_change}
              comparison={proposed}
              tagNames={tagNames}
            />
          </Panel>
        )}
      </div>
      {detail.status === 'pending' && (
        <form
          className="grid gap-4"
          onChange={() => {
            setDirty(true);
            setPreview(undefined);
            setConfirmed(false);
          }}
          onSubmit={(event) => {
            event.preventDefault();
            setFormError('');
            const d = new FormData(event.currentTarget);
            const text = (key: string) => String(d.get(key) ?? '');
            let value: ContributionChange = structuredClone(proposed);
            if (editing) {
              if (detail.kind === 'add_source')
                value = {
                  source: {
                    url: text('url'),
                    label: text('label') || null,
                    source_type: text('source_type') as NonNullable<
                      ContributionChange['source']
                    >['source_type'],
                  },
                };
              if (detail.kind === 'add_tag')
                value = { tag_ids: d.getAll('tag').map(String).sort() };
              if (detail.kind === 'add_relation')
                value = {
                  relation: {
                    ...proposed.relation!,
                    relation_type: text('relation_type') as NonNullable<
                      ContributionChange['relation']
                    >['relation_type'],
                  },
                };
              if (detail.kind === 'add_translation')
                value = {
                  translation: {
                    locale: proposed.translation!.locale,
                    name: text('name'),
                    summary: text('summary') || null,
                    description: text('description') || null,
                  },
                };
            }
            if (value.source)
              value.source.availability_state = text('availability') as NonNullable<
                ContributionChange['source']
              >['availability_state'];
            if (value.translation) {
              const t = value.translation;
              value.translation = {
                locale: t.locale,
                name: t.name,
                summary: t.summary ?? null,
                description: t.description ?? null,
              };
            }
            if (value.tag_ids && (!value.tag_ids.length || value.tag_ids.length > 10)) {
              setFormError('Choose 1 to 10 tags.');
              return;
            }
            setPreview(value);
          }}
        >
          <fieldset disabled={blocked} className="grid gap-4">
            <label>
              <input
                type="checkbox"
                checked={editing}
                onChange={(e) => setEditing(e.target.checked)}
              />{' '}
              Revise before accepting (explain changes below)
            </label>
            {editing && detail.kind === 'add_source' && (
              <>
                <Field label="URL">
                  <input
                    name="url"
                    type="url"
                    required
                    maxLength={2048}
                    defaultValue={proposed.source?.url}
                  />
                </Field>
                <Field label="Label">
                  <input name="label" maxLength={80} defaultValue={proposed.source?.label ?? ''} />
                </Field>
                <Field label="Source type">
                  <select
                    name="source_type"
                    defaultValue={proposed.source?.source_type ?? 'unknown'}
                  >
                    {sourceTypes.map((v) => (
                      <option key={v}>{v}</option>
                    ))}
                  </select>
                </Field>
              </>
            )}
            {detail.kind === 'add_source' && (
              <Field label="Confirm source availability">
                <select name="availability" required defaultValue="">
                  <option value="">Select an assessed state</option>
                  {['active', 'unavailable', 'broken'].map((v) => (
                    <option key={v}>{v}</option>
                  ))}
                </select>
              </Field>
            )}
            {editing && detail.kind === 'add_tag' && (
              <fieldset>
                <legend>Tags to add — preserve unrelated bindings</legend>
                {tags.data?.items
                  .filter((t) => t.state === 'active')
                  .map((t) => (
                    <label className="block" key={t.id}>
                      <input
                        type="checkbox"
                        name="tag"
                        value={t.id}
                        defaultChecked={proposed.tag_ids?.includes(t.id)}
                      />{' '}
                      {t.name}
                    </label>
                  ))}
                {tags.error && <p role="alert">{errorMessage(tags.error)}</p>}
              </fieldset>
            )}
            {editing && detail.kind === 'add_relation' && (
              <Field label="Relation type (endpoints and direction are fixed)">
                <select name="relation_type" defaultValue={proposed.relation?.relation_type}>
                  {(proposed.relation?.relation_type === 'related_to'
                    ? ['related_to']
                    : ['part_of', 'successor_of', 'derived_from']
                  ).map((v) => (
                    <option key={v}>{v}</option>
                  ))}
                </select>
              </Field>
            )}
            {editing && detail.kind === 'add_translation' && (
              <>
                {(['name', 'summary', 'description'] as const).map((key) => (
                  <Field key={key} label={key}>
                    <textarea
                      name={key}
                      required={key === 'name'}
                      rows={key === 'description' ? 10 : 3}
                      defaultValue={proposed.translation?.[key] ?? ''}
                    />
                  </Field>
                ))}
              </>
            )}
            <Field label="Message to author / revision or rejection reason">
              <textarea value={message} onChange={(e) => setMessage(e.target.value)} rows={3} />
            </Field>
            <Field label="Private note">
              <textarea value={note} onChange={(e) => setNote(e.target.value)} rows={3} />
            </Field>
            <Button type="submit">Preview final change</Button>
            <Button
              type="button"
              disabled={!message.trim()}
              onClick={() => {
                if (window.confirm('Reject this proposal with the message shown?'))
                  mutation.mutate('reject');
              }}
            >
              Reject proposal
            </Button>
          </fieldset>
          {preview && (
            <Panel title="Final change — verify before accepting">
              <ChangeReviewSnapshot value={preview} comparison={proposed} tagNames={tagNames} />
              <label>
                <input
                  type="checkbox"
                  checked={confirmed}
                  onChange={(e) => {
                    e.stopPropagation();
                    setConfirmed(e.target.checked);
                  }}
                />{' '}
                I have checked the final change
              </label>
              <Button
                type="button"
                disabled={blocked || !confirmed}
                onClick={() => mutation.mutate('accept')}
              >
                Accept proposal
              </Button>
            </Panel>
          )}
          {(formError || mutation.error) && (
            <p role="alert">{formError || errorMessage(mutation.error)}</p>
          )}
        </form>
      )}
      <Button
        type="button"
        onClick={() => {
          if (!dirty || window.confirm('Discard local changes and reload?')) void reload();
        }}
      >
        Reload canonical snapshot
      </Button>
      {!!detail.resource_changes?.length && (
        <Panel title="Resource revisions">
          <ul>
            {detail.resource_changes.map((v) => (
              <li key={v.resource_id}>
                {v.resource_id}: {v.before_version} → {v.after_version}
              </li>
            ))}
          </ul>
        </Panel>
      )}
      <Panel title="Review history">
        <ol>
          {detail.history.map((e) => (
            <li key={e.event_type}>
              {e.event_type} · {new Date(e.occurred_at).toLocaleString()}
              <p className="whitespace-pre-wrap">{e.message}</p>
              {e.internal_note && (
                <p className="whitespace-pre-wrap">Internal: {e.internal_note}</p>
              )}
            </li>
          ))}
        </ol>
      </Panel>
    </div>
  );
}
export function ChangeReviewSnapshot({
  value,
  comparison,
  tagNames = {},
}: {
  value: ContributionChange;
  comparison?: ContributionChange;
  tagNames?: Record<string, string>;
}) {
  const fields = (v: ContributionChange): Record<string, unknown> => ({
    ...(v.source
      ? {
          url: v.source.url,
          label: v.source.label,
          source_type: v.source.source_type,
          availability: v.source.availability_state,
        }
      : {}),
    ...(v.source_id ? { source_to_remove: v.source_id } : {}),
    ...(v.tag_ids
      ? {
          tags: v.tag_ids.length
            ? v.tag_ids.map((id) => tagNames[id] ?? id).join(', ')
            : 'None of the selected tags is bound',
        }
      : {}),
    ...(v.relation
      ? {
          other_resource: v.relation.other_resource_id,
          type: v.relation.relation_type,
          direction: v.relation.direction,
        }
      : {}),
    ...(v.translation
      ? {
          language: v.translation.locale,
          translation_exists: v.translation.exists,
          name: v.translation.name,
          summary: v.translation.summary,
          description: v.translation.description,
        }
      : {}),
  });
  const base = comparison ? fields(comparison) : undefined;
  return (
    <dl className="grid gap-3 break-words">
      {Object.entries(fields(value)).map(([key, v]) => (
        <div key={key}>
          <dt>
            {key}
            {base && v !== base[key] ? ' · changed' : ''}
          </dt>
          <dd className="whitespace-pre-wrap">{v == null ? '—' : String(v)}</dd>
        </div>
      ))}
    </dl>
  );
}

function ResourceReference({ id, label }: { id: string; label: string }) {
  const resource = useQuery({
    queryKey: ['contribution-resource-reference', id],
    queryFn: async () => result(await getResource(id, readOptions)),
    retry: false,
  });
  return (
    <p>
      {label}:{' '}
      <Link to="/resources/$resourceId" params={{ resourceId: id }}>
        {resource.data?.slug ?? id}
      </Link>
    </p>
  );
}
