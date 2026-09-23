import { useQuery } from '@tanstack/react-query';
import { listCategories, type ContributionContent } from '@tap4furry/api-client/admin';
import { readOptions, result, errorMessage } from '../../lib/admin-api';
import { keys } from '../../lib/query-keys';
import { Field } from '../../components/admin/Primitives';
export function ReviewFields({
  value,
  editing,
}: {
  value: ContributionContent;
  editing: boolean;
}) {
  const categories = useQuery({
    queryKey: keys.categories,
    queryFn: async () => result(await listCategories(readOptions)),
    retry: false,
  });
  return (
    <div className="grid gap-5 sm:grid-cols-2">
      <Field label="Name">
        <input name="name" defaultValue={value.name} required />
      </Field>
      <Field
        label="Slug"
        hint={
          editing
            ? 'Published Resource slugs cannot be changed here.'
            : 'Choose a unique ASCII slug for the new draft.'
        }
      >
        <input
          name="slug"
          defaultValue={value.slug ?? ''}
          required
          pattern="[a-z0-9]+(-[a-z0-9]+)*"
          maxLength={80}
          readOnly={editing}
        />
      </Field>
      <Field label="Default language">
        <input name="locale" defaultValue={value.default_locale} required readOnly={editing} />
      </Field>
      <Field label="Category">
        <select name="category" defaultValue={value.category_id} required>
          {categories.data?.items
            .filter((v) => v.state === 'active' || v.id === value.category_id)
            .map((v) => (
              <option key={v.id} value={v.id}>
                {v.name}
                {v.state === 'retired' ? ' (retired)' : ''}
              </option>
            ))}
        </select>
      </Field>
      <Field label="Content rating">
        <select name="rating" defaultValue={value.content_rating}>
          {['general', 'mature', 'explicit'].map((v) => (
            <option key={v}>{v}</option>
          ))}
        </select>
      </Field>
      <Field label="Lifecycle">
        <select name="lifecycle" defaultValue={value.lifecycle}>
          {['unknown', 'active', 'inactive', 'discontinued', 'delisted', 'archived'].map(
            (v) => (
              <option key={v}>{v}</option>
            ),
          )}
        </select>
      </Field>
      <Field label="Summary">
        <textarea name="summary" defaultValue={value.summary ?? ''} rows={3} />
      </Field>
      <Field label="Description" hint="Plain Markdown, up to 50,000 Unicode characters.">
        <textarea name="description" defaultValue={value.description ?? ''} rows={10} />
      </Field>
      {!editing && (
        <fieldset className="grid gap-4 sm:col-span-2">
          <legend>Optional initial source</legend>
          <Field
            label="URL"
            hint="Leave blank to omit the source. No remote preview is fetched."
          >
            <input
              name="url"
              type="url"
              maxLength={2048}
              defaultValue={value.source?.url ?? ''}
            />
          </Field>
          <Field label="Label">
            <input name="label" defaultValue={value.source?.label ?? ''} />
          </Field>
          <Field label="Type">
            <select name="source_type" defaultValue={value.source?.source_type ?? 'unknown'}>
              {[
                'unknown',
                'official',
                'store',
                'community',
                'mirror',
                'archive',
                'external',
              ].map((v) => (
                <option key={v}>{v}</option>
              ))}
            </select>
          </Field>
          <Field label="Availability">
            <select
              name="availability"
              defaultValue={value.source?.availability_state ?? 'active'}
            >
              {['active', 'unavailable', 'broken', 'removed', 'restricted'].map((v) => (
                <option key={v}>{v}</option>
              ))}
            </select>
          </Field>
          <p>Rights start as unknown. Governance changes stay in the Resource editor.</p>
        </fieldset>
      )}
      {categories.error && <p role="alert">{errorMessage(categories.error)}</p>}
    </div>
  );
}
export function readReview(data: FormData, editing: boolean): ContributionContent {
  const text = (key: string) => String(data.get(key) ?? '');
  const content: ContributionContent = {
    name: text('name'),
    slug: text('slug'),
    default_locale: text('locale'),
    category_id: text('category'),
    summary: text('summary') || null,
    description: text('description') || null,
    lifecycle: text('lifecycle') as ContributionContent['lifecycle'],
    content_rating: text('rating') as ContributionContent['content_rating'],
  };
  if (!editing && text('url'))
    content.source = {
      url: text('url'),
      label: text('label') || null,
      source_type: text('source_type') as NonNullable<
        ContributionContent['source']
      >['source_type'],
      availability_state: text('availability') as NonNullable<
        ContributionContent['source']
      >['availability_state'],
    };
  return content;
}
const contentFields = [
  'name',
  'slug',
  'default_locale',
  'category_id',
  'lifecycle',
  'content_rating',
  'summary',
  'description',
  'source',
] as const;
export function ReviewSnapshot({
  content,
  comparison,
}: {
  content: ContributionContent;
  comparison?: ContributionContent;
}) {
  return (
    <dl className="grid gap-3 break-words">
      {contentFields.map((key) => (
        <div key={key}>
          <dt>
            {key}
            {comparison &&
              JSON.stringify(content[key] ?? null) !==
                JSON.stringify(comparison[key] ?? null) &&
              ' · changed'}
          </dt>
          <dd className="whitespace-pre-wrap">
            {key === 'source'
              ? content.source
                ? `${content.source.url} · ${content.source.label ?? ''} · ${content.source.source_type} · ${content.source.availability_state}`
                : '—'
              : String(content[key] ?? '—')}
          </dd>
        </div>
      ))}
    </dl>
  );
}
