export const keys = {
  me: ['admin', 'me'],
  resources: ['admin', 'resources'],
  resource: (id: string) => ['admin', 'resources', 'detail', id],
  taxonomy: ['admin', 'taxonomy'],
  categories: ['admin', 'taxonomy', 'categories'],
  tags: ['admin', 'taxonomy', 'tags'],
  entity: (kind: string, id: string) => ['admin', 'taxonomy', kind, id],
} as const;
