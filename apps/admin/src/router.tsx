import {
  createRootRoute,
  createRoute,
  createRouter,
  Outlet,
  redirect,
} from '@tanstack/react-router';
import { AdminRequestError, requireAdmin } from './lib/admin-api';
import { AdminShell } from './layouts/AdminShell';
import { LoginPage } from './pages/LoginPage';
import { AccountPage } from './pages/AccountPage';
import { ResourceListPage } from './pages/resources/ResourceListPage';
import { ResourceCreatePage } from './pages/resources/ResourceCreatePage';
import { ResourceLayout } from './pages/resources/ResourceLayout';
import { ResourceOverviewPage } from './pages/resources/ResourceOverviewPage';
import { ResourceLocalizationsPage } from './pages/resources/ResourceLocalizationsPage';
import { ResourceTagsPage } from './pages/resources/ResourceTagsPage';
import { ResourceSourcesPage } from './pages/resources/ResourceSourcesPage';
import { ResourceRelationsPage } from './pages/resources/ResourceRelationsPage';
import { ResourceExternalIDsPage } from './pages/resources/ResourceExternalIDsPage';
import { TaxonomyListPage } from './pages/taxonomy/TaxonomyListPage';
import { TaxonomyPage } from './pages/taxonomy/TaxonomyPage';
import { ContributionListPage } from './pages/contributions/ContributionListPage';
import { ContributionReviewPage } from './pages/contributions/ContributionReviewPage';

const root = createRootRoute({ component: Outlet });
const login = createRoute({
  getParentRoute: () => root,
  path: '/login',
  component: LoginPage,
});
const shell = createRoute({
  getParentRoute: () => root,
  id: 'admin',
  component: AdminShell,
  beforeLoad: async () => {
    try {
      await requireAdmin();
    } catch (error) {
      if (error instanceof AdminRequestError && [401, 403].includes(error.status))
        throw redirect({ to: '/login' });
      throw error;
    }
  },
  errorComponent: () => (
    <main className="p-8">
      <h1>Admin is unavailable</h1>
      <p>Please try again in a moment.</p>
      <a href="/resources">Retry</a>
    </main>
  ),
});
const index = createRoute({
  getParentRoute: () => shell,
  path: '/',
  beforeLoad: () => {
    throw redirect({ to: '/resources' });
  },
});
const resources = createRoute({
  getParentRoute: () => shell,
  path: '/resources',
  component: ResourceListPage,
});
const create = createRoute({
  getParentRoute: () => shell,
  path: '/resources/new',
  component: ResourceCreatePage,
});
const editor = createRoute({
  getParentRoute: () => shell,
  path: '/resources/$resourceId',
  component: ResourceLayout,
});
const overview = createRoute({
  getParentRoute: () => editor,
  path: '/',
  component: ResourceOverviewPage,
});
const localizations = createRoute({
  getParentRoute: () => editor,
  path: '/localizations',
  component: ResourceLocalizationsPage,
});
const tags = createRoute({
  getParentRoute: () => editor,
  path: '/tags',
  component: ResourceTagsPage,
});
const sources = createRoute({
  getParentRoute: () => editor,
  path: '/sources',
  component: ResourceSourcesPage,
});
const relations = createRoute({
  getParentRoute: () => editor,
  path: '/relations',
  component: ResourceRelationsPage,
});
const external = createRoute({
  getParentRoute: () => editor,
  path: '/external-ids',
  component: ResourceExternalIDsPage,
});
const categories = createRoute({
  getParentRoute: () => shell,
  path: '/taxonomy/categories',
  component: () => <TaxonomyListPage kind="categories" />,
});
const category = createRoute({
  getParentRoute: () => shell,
  path: '/taxonomy/categories/$categoryId',
  component: () => <TaxonomyPage kind="categories" />,
});
const taxonomyTags = createRoute({
  getParentRoute: () => shell,
  path: '/taxonomy/tags',
  component: () => <TaxonomyListPage kind="tags" />,
});
const taxonomyTag = createRoute({
  getParentRoute: () => shell,
  path: '/taxonomy/tags/$tagId',
  component: () => <TaxonomyPage kind="tags" />,
});
const account = createRoute({
  getParentRoute: () => shell,
  path: '/account',
  component: AccountPage,
});
const contributions = createRoute({
  getParentRoute: () => shell,
  path: '/contributions',
  component: ContributionListPage,
});
const contribution = createRoute({
  getParentRoute: () => shell,
  path: '/contributions/$contributionId',
  component: ContributionReviewPage,
});
export const router = createRouter({
  routeTree: root.addChildren([
    login,
    shell.addChildren([
      index,
      resources,
      create,
      editor.addChildren([overview, localizations, tags, sources, relations, external]),
      categories,
      category,
      taxonomyTags,
      taxonomyTag,
      account,
      contributions,
      contribution,
    ]),
  ]),
});
declare module '@tanstack/react-router' {
  interface Register {
    router: typeof router;
  }
}
