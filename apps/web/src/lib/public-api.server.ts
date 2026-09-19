import { getGetResourceUrl, getListResourcesUrl } from '@tap4furry/api-client/public';
import type { ResourceDetail, ResourceList } from '@tap4furry/api-client/public';

export const PUBLIC_READ_CACHE = 'public, max-age=0, s-maxage=60, stale-while-revalidate=30';
export type ReadResult<T> = { status: 200; data: T } | { status: 400 | 404 | 503 };
export interface ReadQuery { locale: string; explicitLocale: boolean; page: number }

export function readQuery(url: URL, paged: boolean): ReadQuery | null {
  const explicitLocale = url.searchParams.has('locale');
  const raw = url.searchParams.get('locale') ?? 'en';
  if (url.searchParams.getAll('locale').length > 1 || raw.length < 2 || raw.length > 64 || !/^[A-Za-z0-9]+(-[A-Za-z0-9]+)*$/.test(raw)) return null;
  let locale: string;
  try { [locale] = Intl.getCanonicalLocales(raw); } catch { return null; }
  const rawPage = paged ? url.searchParams.get('page') ?? '1' : '1';
  const page = Number(rawPage);
  if ((paged && url.searchParams.getAll('page').length > 1) || !/^\d+$/.test(rawPage) || !Number.isSafeInteger(page) || page < 1 || !Number.isSafeInteger((page - 1) * 24)) return null;
  return { locale, explicitLocale, page };
}

export function resourceHref(slug: string, query: ReadQuery): string {
  return `/resources/${encodeURIComponent(slug)}${query.explicitLocale ? `?${new URLSearchParams({ locale: query.locale })}` : ''}`;
}

export function listHref(page: number, query: ReadQuery): string {
  const params = new URLSearchParams();
  if (page > 1) params.set('page', String(page));
  if (query.explicitLocale) params.set('locale', query.locale);
  return `/resources${params.size ? `?${params}` : ''}`;
}

// Runtime guards reject invalid upstream JSON before rendering. OpenAPI still
// owns the public shapes; these narrow checks require their renderable fields.
type RecordValue = Record<string, unknown>;
const record = (x: unknown): x is RecordValue => !!x && typeof x === 'object' && !Array.isArray(x);
const text = (x: unknown): x is string => typeof x === 'string';
const optional = (x: unknown) => x === null || text(x);
const nonempty = (x: unknown): x is string => text(x) && x.length > 0;
const id = (x: unknown) => text(x) && /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(x);
const slug = (x: unknown) => text(x) && x.length <= 80 && /^[a-z0-9]+(-[a-z0-9]+)*$/.test(x);
const date = (x: unknown) => text(x) && !Number.isNaN(Date.parse(x));
const oneOf = (x: unknown, choices: string[]) => text(x) && choices.includes(x);
const reference = (x: unknown) => record(x) && id(x.id) && slug(x.slug) && nonempty(x.name);
const arrayOf = (x: unknown, check: (item: unknown) => boolean) => Array.isArray(x) && x.every(check);
const item = (x: unknown): boolean => record(x) && reference(x) && optional(x.summary) && reference(x.category)
  && oneOf(x.lifecycle, ['active', 'inactive', 'discontinued', 'delisted', 'archived', 'unknown'])
  && oneOf(x.content_rating, ['general', 'mature', 'explicit']) && date(x.published_at) && date(x.updated_at);
const source = (x: unknown): boolean => {
  if (!record(x) || !id(x.id) || !text(x.url) || !optional(x.label) || typeof x.is_primary !== 'boolean') return false;
  try { const u = new URL(x.url); if (!['http:', 'https:'].includes(u.protocol) || u.username || u.password) return false; } catch { return false; }
  return oneOf(x.source_type, ['official', 'store', 'archive', 'mirror', 'community', 'external', 'unknown']) && oneOf(x.availability_state, ['active', 'unavailable', 'broken', 'restricted']);
};
function resourceList(x: unknown): x is ResourceList {
  return record(x) && arrayOf(x.items, item) && Number.isSafeInteger(x.page) && Number(x.page) >= 1 && Number.isInteger(x.page_size) && Number(x.page_size) >= 1 && Number(x.page_size) <= 100 && typeof x.has_next === 'boolean';
}
function resourceDetail(x: unknown): x is ResourceDetail {
  return record(x) && item(x) && optional(x.description) && optional(x.requested_locale) && nonempty(x.default_locale)
    && arrayOf(x.available_locales, nonempty) && arrayOf(x.tags, reference) && arrayOf(x.sources, source)
    && arrayOf(x.relations, r => record(r) && oneOf(r.type, ['part_of', 'successor_of', 'derived_from', 'related_to']) && oneOf(r.direction, ['outgoing', 'incoming', 'symmetric']) && reference(r.resource))
    && arrayOf(x.external_ids, r => record(r) && nonempty(r.namespace) && nonempty(r.external_id));
}

async function read<T>(path: string, valid: (data: unknown) => data is T, detail: boolean): Promise<ReadResult<T>> {
  try {
    const configured = process.env.API_INTERNAL_ORIGIN || (import.meta.env.DEV ? 'http://127.0.0.1:8080' : '');
    const origin = new URL(configured);
    if (!['http:', 'https:'].includes(origin.protocol) || origin.username || origin.password || origin.pathname !== '/' || origin.search || origin.hash || !path.startsWith('/api/')) return { status: 503 };
    const response = await fetch(new URL(path.slice(4), origin), {
      method: 'GET', headers: { Accept: 'application/json' }, credentials: 'omit', redirect: 'error',
      signal: globalThis.AbortSignal.timeout(5000),
    });
    if (response.status === 400) return { status: 400 };
    if (detail && response.status === 404) return { status: 404 };
    if (response.status !== 200 || !response.headers.get('content-type')?.includes('application/json')) return { status: 503 };
    const data: unknown = await response.json();
    return valid(data) ? { status: 200, data } : { status: 503 };
  } catch { return { status: 503 }; }
}

export function listResources(query: ReadQuery): Promise<ReadResult<ResourceList>> {
  return read(getListResourcesUrl({ locale: query.locale, page: query.page, page_size: 24 }), resourceList, false);
}
export function getResource(slug: string, query: ReadQuery): Promise<ReadResult<ResourceDetail>> {
  if (!/^[a-z0-9]+(-[a-z0-9]+)*$/.test(slug) || slug.length > 80) return Promise.resolve({ status: 404 });
  return read(getGetResourceUrl(slug, { locale: query.locale }), resourceDetail, true);
}
