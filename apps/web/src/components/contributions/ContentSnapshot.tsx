import type { ContributionOriginal } from '@tap4furry/api-client/public';
export function ContentSnapshot({
  content,
  categoryName,
}: {
  content: ContributionOriginal;
  categoryName?: string;
}) {
  return (
    <dl className="grid gap-3 break-words">
      {content.name !== undefined && (
        <div>
          <dt>Name</dt>
          <dd>{content.name}</dd>
        </div>
      )}
      {(content.default_locale || content.content_rating || content.lifecycle) && (
        <div>
          <dt>Language / rating / lifecycle</dt>
          <dd>
            {content.default_locale} · {content.content_rating} · {content.lifecycle}
          </dd>
        </div>
      )}
      {content.category_id !== undefined && (
        <div>
          <dt>Category</dt>
          <dd>{categoryName ?? 'Category recorded in your proposal'}</dd>
        </div>
      )}
      {content.summary !== undefined && (
        <div>
          <dt>Summary</dt>
          <dd className="whitespace-pre-wrap">{content.summary || '—'}</dd>
        </div>
      )}
      {content.description !== undefined && (
        <div>
          <dt>Description</dt>
          <dd className="whitespace-pre-wrap">{content.description || '—'}</dd>
        </div>
      )}
      {content.source && (
        <div>
          <dt>Initial source</dt>
          <dd>
            {content.source.url} · {content.source.label} · {content.source.source_type}
          </dd>
        </div>
      )}
    </dl>
  );
}
