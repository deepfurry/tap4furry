import type { ContributionChange } from '@tap4furry/api-client/public';

export const changeNames: Record<string, string> = {
  create_resource: 'New Resource',
  update_resource: 'Basic correction',
  add_source: 'Add a source',
  remove_broken_source: 'Remove a broken source',
  add_tag: 'Add tags',
  add_relation: 'Add a relation',
  add_translation: 'Add or correct a translation',
};
export function ChangeSnapshot({
  value,
  tagNames = {},
}: {
  value: ContributionChange;
  tagNames?: Record<string, string>;
}) {
  return (
    <div className="grid gap-3 break-words">
      {value.source && (
        <>
          <p>{value.source.url}</p>
          <p>
            {value.source.label || 'No label'} · {value.source.source_type || 'unknown'}
          </p>
        </>
      )}
      {value.source_id && !value.source && (
        <p>Remove the selected source from public display. Its record is retained.</p>
      )}
      {value.tag_ids && (
        <ul>
          {value.tag_ids.map((id) => (
            <li key={id}>{tagNames[id] ?? 'Selected tag — current name unavailable'}</li>
          ))}
        </ul>
      )}
      {value.relation && (
        <p>
          {value.relation.relation_type} · {value.relation.direction} · Other Resource ID:{' '}
          {value.relation.other_resource_id}
        </p>
      )}
      {value.translation && (
        <dl className="grid gap-3">
          <div>
            <dt>Language</dt>
            <dd>{value.translation.locale}</dd>
          </div>
          {(['name', 'summary', 'description'] as const).map(
            (key) =>
              key in value.translation! && (
                <div key={key}>
                  <dt>{key}</dt>
                  <dd className="whitespace-pre-wrap">
                    {value.translation![key] ?? 'Cleared — use the default-language fallback'}
                  </dd>
                </div>
              ),
          )}
        </dl>
      )}
    </div>
  );
}
