import { useEffect, useState } from 'react';
import {
  getMe,
  getResource,
  listTags,
  getContributionContext,
  getMyContribution,
  submitContribution,
  type ContributionKind,
  type ContributionContext,
  type ContributionChange,
  type ResourceDetail,
  type TagItem,
  type GetContributionContextParams,
} from '@tap4furry/api-client/public';
import { authenticatedRequest } from '../../lib/security';
import {
  checked,
  privateRead,
  contributionError,
  useContributionDirty,
  ContributionError,
} from '../../lib/contributions';
import { ChangeSnapshot, changeNames } from './ChangeSnapshot';
import styles from '../Account.module.scss';

export function ChangeSubmitForm({
  kind,
  slug,
  previous,
}: {
  kind: ContributionKind;
  slug: string;
  previous?: string;
}) {
  const [resource, setResource] = useState<ResourceDetail>();
  const [tags, setTags] = useState<TagItem[]>([]);
  const [verified, setVerified] = useState(false);
  const [context, setContext] = useState<ContributionContext>();
  const [old, setOld] = useState<ContributionChange>();
  const [error, setError] = useState('');
  const [conflict, setConflict] = useState(false);
  const [busy, setBusy] = useState(false);
  const [dirty, setDirty] = useState(false);
  const [preview, setPreview] = useState<{
    change: ContributionChange;
    reason: string;
  }>();
  const [request, setRequest] = useState<{ body: string; id: string }>();
  const [created, setCreated] = useState('');
  useContributionDirty(dirty);
  useEffect(() => {
    let active = true;
    void (async () => {
      try {
        const me = checked(await getMe(privateRead));
        const record = checked(await getResource(slug, undefined, privateRead));
        const available =
          kind === 'add_tag' ? checked(await listTags(undefined, privateRead)).items : [];
        const oldRecord = previous
          ? checked(await getMyContribution(previous, privateRead))
          : undefined;
        if (active) {
          setVerified(me.email_verified);
          setResource(record);
          setTags(available);
          setOld(oldRecord?.proposed_change);
        }
        if (kind === 'add_source' || kind === 'add_tag') {
          const value = checked(await getContributionContext(slug, { kind }, privateRead));
          if (active) setContext(value);
        }
      } catch (e) {
        if (active) setError(contributionError(e));
      }
    })();
    return () => {
      active = false;
    };
  }, [kind, slug, previous]);
  if (created)
    return (
      <p role="status">
        Proposal submitted. <a href={`/contributions/${created}`}>View your proposal</a>
      </p>
    );
  const source = context?.change?.source;
  const translation = context?.change?.translation;
  return (
    <div className="grid gap-5">
      <h2>{changeNames[kind]}</h2>
      {error && <p role="alert">{error}</p>}
      {!resource ? (
        <p role="status">Loading Resource…</p>
      ) : !verified ? (
        <p>
          Verify your email from <a href="/account">Account</a> before submitting.
        </p>
      ) : (
        <>
          <p>Resource: {resource.name}</p>
          {old && (
            <details>
              <summary>Previous original — reference only</summary>
              <ChangeSnapshot
                tagNames={Object.fromEntries(tags.map((t) => [t.id, t.name]))}
                value={old}
              />
              <p>Start from the current record and explicitly re-enter your changes.</p>
            </details>
          )}
          {!context ? (
            <form
              className="grid gap-4"
              onSubmit={async (event) => {
                event.preventDefault();
                setBusy(true);
                setError('');
                const data = new FormData(event.currentTarget);
                const params: GetContributionContextParams = { kind };
                if (kind === 'remove_broken_source') params.source_id = String(data.get('source'));
                if (kind === 'add_translation') params.locale = String(data.get('locale'));
                if (kind === 'add_relation') {
                  params.other_slug = String(data.get('other_slug'));
                  params.relation_type = String(
                    data.get('relation_type'),
                  ) as GetContributionContextParams['relation_type'];
                  params.direction =
                    params.relation_type === 'related_to'
                      ? 'symmetric'
                      : (String(
                          data.get('direction'),
                        ) as GetContributionContextParams['direction']);
                }
                try {
                  setContext(checked(await getContributionContext(slug, params, privateRead)));
                } catch (e) {
                  setError(contributionError(e));
                } finally {
                  setBusy(false);
                }
              }}
            >
              {kind === 'remove_broken_source' && (
                <label>
                  Source to remove
                  <select className={styles.input} name="source" required>
                    <option value="">Choose a source</option>
                    {resource.sources.map((s) => (
                      <option value={s.id} key={s.id}>
                        {s.label || s.url}
                      </option>
                    ))}
                  </select>
                </label>
              )}
              {kind === 'add_translation' && (
                <label>
                  Translation language
                  <input
                    className={styles.input}
                    name="locale"
                    placeholder="e.g. zh-CN"
                    required
                    maxLength={64}
                  />
                </label>
              )}
              {kind === 'add_relation' && (
                <>
                  <label>
                    Other Resource slug
                    <input className={styles.input} name="other_slug" required maxLength={80} />
                  </label>
                  <label>
                    Relation
                    <select className={styles.input} name="relation_type">
                      {['part_of', 'successor_of', 'derived_from', 'related_to'].map((t) => (
                        <option key={t}>{t}</option>
                      ))}
                    </select>
                  </label>
                  <label>
                    Direction (ignored for related_to)
                    <select className={styles.input} name="direction">
                      <option value="outgoing">This Resource → other Resource</option>
                      <option value="incoming">Other Resource → this Resource</option>
                    </select>
                  </label>
                </>
              )}
              <button className={styles.button} disabled={busy}>
                Load current context
              </button>
            </form>
          ) : (
            <form
              className="grid gap-5"
              onChange={() => {
                setDirty(true);
                setPreview(undefined);
              }}
              onSubmit={(event) => {
                event.preventDefault();
                setError('');
                const data = new FormData(event.currentTarget);
                const text = (key: string) => String(data.get(key) ?? '');
                const change: ContributionChange = {};
                if (kind === 'add_source')
                  change.source = {
                    url: text('url'),
                    label: text('label') || null,
                    source_type: text('source_type') as NonNullable<
                      ContributionChange['source']
                    >['source_type'],
                  };
                if (kind === 'remove_broken_source') change.source_id = context.change!.source_id;
                if (kind === 'add_tag') {
                  change.tag_ids = data.getAll('tag').map(String).sort();
                  if (change.tag_ids.length < 1 || change.tag_ids.length > 10) {
                    setError('Choose between 1 and 10 tags.');
                    return;
                  }
                }
                if (kind === 'add_relation') change.relation = context.change!.relation;
                if (kind === 'add_translation') {
                  change.translation = { locale: translation!.locale };
                  for (const key of ['name', 'summary', 'description'] as const) {
                    const mode = translation?.exists ? text(`${key}_mode`) : 'change';
                    if (mode === 'keep') continue;
                    if (key === 'name') {
                      if (!text(key)) {
                        setError('A translation name is required.');
                        return;
                      }
                      change.translation.name = text(key);
                    } else change.translation[key] = mode === 'clear' ? null : text(key) || null;
                  }
                }
                setPreview({ change, reason: text('reason') });
              }}
            >
              <fieldset disabled={busy} className="grid gap-4">
                {kind === 'add_source' && (
                  <>
                    <label className="grid gap-2">
                      URL
                      <input
                        className={styles.input}
                        name="url"
                        type="url"
                        required
                        maxLength={2048}
                      />
                    </label>
                    <label className="grid gap-2">
                      Label
                      <input className={styles.input} name="label" maxLength={80} />
                    </label>
                    <label className="grid gap-2">
                      Source type
                      <select className={styles.input} name="source_type">
                        {[
                          'unknown',
                          'official',
                          'store',
                          'archive',
                          'mirror',
                          'community',
                          'external',
                        ].map((t) => (
                          <option key={t}>{t}</option>
                        ))}
                      </select>
                    </label>
                    <p>No automatic URL fetching or rights confirmation is performed.</p>
                  </>
                )}
                {kind === 'remove_broken_source' && (
                  <p>
                    Remove {source?.label || source?.url} from public display. The original record
                    is retained.
                  </p>
                )}
                {kind === 'add_tag' && (
                  <fieldset>
                    <legend>Choose up to 10 tags</legend>
                    <div className="grid gap-2 sm:grid-cols-2">
                      {tags
                        .filter((t) => !resource.tags.some((bound) => bound.id === t.id))
                        .map((t) => (
                          <label key={t.id}>
                            <input type="checkbox" name="tag" value={t.id} /> {t.name}
                          </label>
                        ))}
                    </div>
                  </fieldset>
                )}
                {kind === 'add_relation' && (
                  <p>
                    {context.change?.relation?.direction === 'incoming'
                      ? context.other_resource?.name
                      : resource.name}{' '}
                    — {context.change?.relation?.relation_type} →{' '}
                    {context.change?.relation?.direction === 'incoming'
                      ? resource.name
                      : context.other_resource?.name}
                  </p>
                )}
                {kind === 'add_translation' && (
                  <>
                    <p>
                      Language: {translation?.locale} ·{' '}
                      {translation?.exists ? 'Edit existing translation' : 'New translation'}
                    </p>
                    <details>
                      <summary>Default-language reference — not part of your translation</summary>
                      <p>{context.reference?.name}</p>
                      <p className="whitespace-pre-wrap">{context.reference?.summary}</p>
                      <p className="whitespace-pre-wrap">{context.reference?.description}</p>
                    </details>
                    {(['name', 'summary', 'description'] as const).map((key) => (
                      <label className="grid gap-2" key={key}>
                        {key}
                        {translation?.exists && (
                          <select className={styles.input} name={`${key}_mode`} defaultValue="keep">
                            <option value="keep">Keep current value</option>
                            <option value="change">Change</option>
                            {key !== 'name' && (
                              <option value="clear">Clear and use fallback</option>
                            )}
                          </select>
                        )}
                        <textarea
                          className={styles.input}
                          name={key}
                          defaultValue={translation?.[key] ?? ''}
                          rows={key === 'description' ? 8 : 2}
                        />
                      </label>
                    ))}
                  </>
                )}
                <label className="grid gap-2">
                  Reason and supporting information
                  <textarea className={styles.input} name="reason" required rows={4} />
                </label>
                <button className={styles.button}>Preview proposal</button>
              </fieldset>
              {preview && (
                <section className="grid gap-3">
                  <h3>Confirm your proposal</h3>
                  <ChangeSnapshot
                    tagNames={Object.fromEntries(tags.map((t) => [t.id, t.name]))}
                    value={preview.change}
                  />
                  <p className="whitespace-pre-wrap">{preview.reason}</p>
                  <button
                    type="button"
                    className={styles.button}
                    disabled={busy}
                    onClick={async () => {
                      const body = {
                        kind,
                        reason: preview.reason,
                        change: preview.change,
                        target_resource_id: context.resource_id,
                        base_revision: context.base_revision,
                        ...(previous ? { previous_id: previous } : {}),
                      };
                      const serialized = JSON.stringify(body),
                        id = request?.body === serialized ? request.id : crypto.randomUUID();
                      setRequest({ body: serialized, id });
                      setBusy(true);
                      setError('');
                      try {
                        const result = checked(
                          await submitContribution(
                            { ...body, request_id: id },
                            await authenticatedRequest(),
                          ),
                        );
                        setDirty(false);
                        setCreated(result.id);
                      } catch (e) {
                        setError(contributionError(e));
                        setConflict(e instanceof ContributionError && e.status === 409);
                      } finally {
                        setBusy(false);
                      }
                    }}
                  >
                    Submit for review
                  </button>
                </section>
              )}
            </form>
          )}
          {(context || conflict) && (
            <button
              type="button"
              onClick={() => {
                if (window.confirm('Discard this form and reload the current Resource?'))
                  window.location.reload();
              }}
            >
              Reload current record
            </button>
          )}
          {conflict && (
            <p>
              Your input is preserved. Review the conflict before explicitly reloading or
              resubmitting.
            </p>
          )}
        </>
      )}
    </div>
  );
}
