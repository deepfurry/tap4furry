import type { ContributionOriginal, CategoryItem } from '@tap4furry/api-client/public';
import styles from '../Account.module.scss';
export function ContentFields({
  value,
  categories,
  editing,
}: {
  value?: ContributionOriginal;
  categories: CategoryItem[];
  editing: boolean;
}) {
  return (
    <div className="grid gap-4">
      <label className="grid gap-2">
        Name
        <input className={styles.input} name="name" required defaultValue={value?.name} />
      </label>
      <label className="grid gap-2">
        Default language
        <input
          className={styles.input}
          name="locale"
          required
          defaultValue={value?.default_locale ?? 'en'}
          readOnly={editing}
        />
      </label>
      <label className="grid gap-2">
        Category
        <select
          className={styles.input}
          name="category"
          required
          defaultValue={value?.category_id ?? ''}
        >
          <option value="">Choose a category</option>
          {value && !categories.some((v) => v.id === value.category_id) && (
            <option value={value.category_id}>Keep current category</option>
          )}
          {categories.map((v) => (
            <option key={v.id} value={v.id}>
              {v.name}
            </option>
          ))}
        </select>
      </label>
      <label className="grid gap-2">
        Content rating
        <select
          className={styles.input}
          name="rating"
          required
          defaultValue={value?.content_rating ?? ''}
        >
          <option value="">Choose a rating</option>
          {['general', 'mature', 'explicit'].map((v) => (
            <option key={v}>{v}</option>
          ))}
        </select>
      </label>
      <label className="grid gap-2">
        Lifecycle
        <select
          className={styles.input}
          name="lifecycle"
          defaultValue={value?.lifecycle ?? 'unknown'}
        >
          {['unknown', 'active', 'inactive', 'discontinued', 'delisted', 'archived'].map(
            (v) => (
              <option key={v}>{v}</option>
            ),
          )}
        </select>
      </label>
      <label className="grid gap-2">
        Summary
        <textarea
          className={styles.input}
          name="summary"
          rows={3}
          defaultValue={value?.summary ?? ''}
        />
      </label>
      <label className="grid gap-2">
        Description (Markdown)
        <textarea
          className={styles.input}
          name="description"
          rows={10}
          defaultValue={value?.description ?? ''}
        />
      </label>
      <p className={styles.hint}>
        Name: 160 characters. Summary: 500. Description: 50,000. Plain Markdown only; no images
        or HTML.
      </p>
      {!editing && (
        <fieldset className="grid gap-4">
          <legend>Initial source (optional)</legend>
          <label className="grid gap-2">
            Source URL
            <input
              className={styles.input}
              name="url"
              type="url"
              maxLength={2048}
              defaultValue={value?.source?.url}
            />
          </label>
          <label className="grid gap-2">
            Source label
            <input
              className={styles.input}
              name="label"
              defaultValue={value?.source?.label ?? ''}
            />
          </label>
          <label className="grid gap-2">
            Source type
            <select
              className={styles.input}
              name="source_type"
              defaultValue={value?.source?.source_type ?? 'unknown'}
            >
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
          </label>
          <p className={styles.hint}>
            Provide a summary if there is no source. URLs are not fetched or previewed.
          </p>
        </fieldset>
      )}
    </div>
  );
}
