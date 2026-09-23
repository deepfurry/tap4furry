import { useEffect, useState } from 'react';
import {
  getMe,
  listCategories,
  getContributionContext,
  getMyContribution,
  submitContribution,
  type ContributionKind,
  type CategoryItem,
  type ContributionContext,
  type ContributionContent,
  type ContributionOriginal,
  type SubmitContribution,
} from '@tap4furry/api-client/public';
import { authenticatedRequest } from '../../lib/security';
import {
  checked,
  privateRead,
  contributionError,
  useContributionDirty,
  ContributionError,
} from '../../lib/contributions';
import { ContentFields } from './ContentFields';
import { ContentSnapshot } from './ContentSnapshot';
import styles from '../Account.module.scss';
import { ChangeSubmitForm } from './ChangeSubmitForm';
import { changeNames } from './ChangeSnapshot';
type Initial = {
  categories: CategoryItem[];
  verified: boolean;
  context?: ContributionContext;
  content?: ContributionOriginal;
  previous?: string;
  oldContent?: ContributionOriginal;
  reason: string;
};
export function SubmitForm() {
  const [route, setRoute] = useState<{
    kind: ContributionKind;
    slug: string;
    previous?: string;
  }>();
  useEffect(() => {
    const q = new URLSearchParams(window.location.search);
    const slug = q.get('resource') ?? '';
    setRoute({
      kind: (q.get('kind') ?? (slug ? 'update_resource' : 'create_resource')) as ContributionKind,
      slug,
      previous: q.get('previous') ?? undefined,
    });
  }, []);
  if (!route) return <p role="status">Loading contribution form…</p>;
  const extended = !['create_resource', 'update_resource'].includes(route.kind);
  if (!changeNames[route.kind] || (extended && !route.slug))
    return <p>Open a public Resource to suggest this change.</p>;
  return (
    <div className="grid gap-6">
      {route.slug && (
        <nav aria-label="Contribution type" className="flex flex-wrap gap-4">
          {Object.entries(changeNames)
            .filter(([kind]) => kind !== 'create_resource')
            .map(([kind, label]) => (
              <a
                key={kind}
                aria-current={route.kind === kind ? 'page' : undefined}
                href={'/submit?resource=' + encodeURIComponent(route.slug) + '&kind=' + kind}
              >
                {label}
              </a>
            ))}
        </nav>
      )}
      {extended ? (
        <ChangeSubmitForm
          key={route.kind}
          kind={route.kind}
          slug={route.slug}
          previous={route.previous}
        />
      ) : (
        <BasicSubmitForm />
      )}
    </div>
  );
}
function BasicSubmitForm() {
  const [initial, setInitial] = useState<Initial>();
  const [error, setError] = useState('');
  const [conflict, setConflict] = useState(false);
  const [busy, setBusy] = useState(false);
  const [dirty, setDirty] = useState(false);
  const [request, setRequest] = useState<{ body: string; id: string }>();
  const [created, setCreated] = useState<string>();
  useContributionDirty(dirty);
  useEffect(() => {
    let active = true;
    void (async () => {
      try {
        const me = checked(await getMe(privateRead));
        const categories = checked(await listCategories(undefined, privateRead)).items;
        const query = new URLSearchParams(window.location.search);
        const slug = query.get('resource');
        const previous = query.get('previous');
        const context = slug
          ? checked(await getContributionContext(slug, undefined, privateRead))
          : undefined;
        const old = previous ? checked(await getMyContribution(previous, privateRead)) : undefined;
        if (old?.kind === 'update_resource' && !context)
          throw new ContributionError(
            409,
            'Open the current Resource and choose Suggest a correction to submit again. Your previous proposal remains in your history.',
          );
        if (active)
          setInitial({
            categories,
            verified: me.email_verified,
            context,
            content: context?.content ?? old?.proposed,
            oldContent: context && old ? old.proposed : undefined,
            previous: old?.id,
            reason: old?.reason ?? '',
          });
      } catch (err) {
        if (active) setError(contributionError(err));
      }
    })();
    return () => {
      active = false;
    };
  }, []);
  if (created)
    return (
      <div role="status" className="grid gap-4">
        <h2>Proposal submitted</h2>
        <p>A reviewer will check it before it changes the catalog.</p>
        <a href={`/contributions/${created}`}>View your proposal</a>
      </div>
    );
  if (!initial)
    return <p role={error ? 'alert' : 'status'}>{error || 'Loading submission form…'}</p>;
  if (!initial.verified)
    return (
      <p>
        Verify your email from <a href="/account">Account</a> before submitting a proposal.
      </p>
    );
  return (
    <form
      className="grid gap-5"
      onChange={() => setDirty(true)}
      onSubmit={async (event) => {
        event.preventDefault();
        setBusy(true);
        setError('');
        const data = new FormData(event.currentTarget);
        const text = (key: string) => String(data.get(key) ?? '');
        const content: SubmitContribution['content'] = {
          default_locale: text('locale'),
          category_id: text('category'),
          name: text('name'),
          summary: text('summary') || null,
          description: text('description') || null,
          lifecycle: text('lifecycle') as ContributionContent['lifecycle'],
          content_rating: text('rating') as ContributionContent['content_rating'],
        };
        if (!initial.context && text('url'))
          content.source = {
            url: text('url'),
            label: text('label') || null,
            source_type: text('source_type') as NonNullable<
              ContributionContent['source']
            >['source_type'],
          };
        const body = {
          kind: initial.context ? 'update_resource' : 'create_resource',
          reason: text('reason'),
          content,
          ...(initial.context && {
            target_resource_id: initial.context.resource_id,
            base_revision: initial.context.base_revision,
          }),
          ...(initial.previous && { previous_id: initial.previous }),
        } as Omit<SubmitContribution, 'request_id'>;
        const serialized = JSON.stringify(body);
        const id = request?.body === serialized ? request.id : crypto.randomUUID();
        setRequest({ body: serialized, id });
        try {
          const result = checked(
            await submitContribution({ ...body, request_id: id }, await authenticatedRequest()),
          );
          setDirty(false);
          setCreated(result.id);
        } catch (err) {
          setError(contributionError(err));
          setConflict(err instanceof ContributionError && err.status === 409);
        } finally {
          setBusy(false);
        }
      }}
    >
      <p>
        {initial.context
          ? 'Suggest a correction to the current default-language record.'
          : 'Suggest a new Resource. Accepted proposals start as drafts.'}
      </p>
      {initial.oldContent && (
        <details>
          <summary>Previous proposal (reference only)</summary>
          <p>
            The form starts from current canonical content. Reapply only the changes you still want
            to suggest.
          </p>
          <ContentSnapshot
            content={initial.oldContent}
            categoryName={
              initial.categories.find((v) => v.id === initial.oldContent?.category_id)?.name
            }
          />
        </details>
      )}
      <fieldset disabled={busy} className="grid gap-5">
        <ContentFields
          value={initial.content}
          categories={initial.categories}
          editing={Boolean(initial.context)}
        />
        <label className="grid gap-2">
          Reason and supporting information
          <textarea
            className={styles.input}
            name="reason"
            required
            rows={4}
            defaultValue={initial.reason}
          />
        </label>
        <button className={styles.button}>Submit for review</button>
      </fieldset>
      {error && <p role="alert">{error}</p>}
      {conflict && (
        <p>
          The Resource or request has changed. Your form is preserved.{' '}
          <button
            type="button"
            onClick={() => {
              if (window.confirm('Discard this form and load the current Resource?'))
                window.location.reload();
            }}
          >
            Reload current record
          </button>
        </p>
      )}
    </form>
  );
}
