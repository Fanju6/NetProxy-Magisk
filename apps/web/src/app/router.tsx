import {
  createHashHistory,
  createRootRoute,
  createRoute,
  createRouter,
  lazyRouteComponent,
} from '@tanstack/react-router'
import { AppShell } from './shell'
import { DashboardPage } from '@/features/dashboard'

const root = createRootRoute({ component: AppShell })
const routeTree = root.addChildren([
  createRoute({ getParentRoute: () => root, path: '/', component: DashboardPage }),
  createRoute({
    getParentRoute: () => root,
    path: '/nodes',
    component: lazyRouteComponent(() => import('@/features/nodes'), 'NodesPage'),
  }),
  createRoute({
    getParentRoute: () => root,
    path: '/subscriptions',
    component: lazyRouteComponent(() => import('@/features/subscriptions'), 'SubscriptionsPage'),
  }),
  createRoute({
    getParentRoute: () => root,
    path: '/settings',
    component: lazyRouteComponent(() => import('@/features/settings'), 'SettingsPage'),
  }),
  createRoute({
    getParentRoute: () => root,
    path: '/settings/configuration',
    component: lazyRouteComponent(() => import('@/features/configuration'), 'ConfigurationPage'),
  }),
  createRoute({
    getParentRoute: () => root,
    path: '/settings/policies',
    component: lazyRouteComponent(() => import('@/features/policies'), 'PoliciesPage'),
  }),
  createRoute({
    getParentRoute: () => root,
    path: '/settings/logs',
    component: lazyRouteComponent(() => import('@/features/operations'), 'OperationsPage'),
  }),
])
export function makeRouter() {
  return createRouter({
    routeTree,
    history: createHashHistory(),
    defaultPreload: 'intent',
    scrollRestoration: true,
  })
}
declare module '@tanstack/react-router' {
  interface Register {
    router: ReturnType<typeof makeRouter>
  }
}
