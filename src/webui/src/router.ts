import { createRouter, createWebHashHistory } from 'vue-router';

const Dashboard = () => import('./components/DashboardScreen.vue');
const Nodes = () => import('./components/NodesScreen.vue');
const Subscriptions = () => import('./components/SubscriptionsScreen.vue');
const SubscriptionDetails = () => import('./components/SubscriptionDetailsScreen.vue');
const SubscriptionEditor = () => import('./components/SubscriptionEditorScreen.vue');
const SettingsLayout = () => import('./components/SettingsLayout.vue');
const SettingsMain = () => import('./components/SettingsMain.vue');
const Apps = () => import('./components/AppsScreen.vue');
const ProxySettings = () => import('./components/ProxySettingsScreen.vue');
const LogsSettings = () => import('./components/LogsScreen.vue');
const AboutSettings = () => import('./components/AboutScreen.vue');

export const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    { path: '/', redirect: '/dashboard' },
    { path: '/dashboard', name: 'dashboard', component: Dashboard },
    { path: '/nodes', name: 'nodes', component: Nodes },
    { path: '/subscriptions', name: 'subscriptions', component: Subscriptions },
    {
      path: '/subscriptions/new',
      name: 'subscription-new',
      component: SubscriptionEditor,
      props: { id: 'new' },
      meta: { showBack: true }
    },
    {
      path: '/subscriptions/:id',
      name: 'subscription-details',
      component: SubscriptionDetails,
      props: true,
      meta: { showBack: true }
    },
    {
      path: '/subscriptions/:id/edit',
      name: 'subscription-edit',
      component: SubscriptionEditor,
      props: true,
      meta: { showBack: true }
    },
    {
      path: '/settings',
      component: SettingsLayout,
      children: [
        { path: '', name: 'settings-main', component: SettingsMain },
        { path: 'apps', name: 'settings-apps', component: Apps, meta: { showBack: true } },
        { path: 'proxy', name: 'settings-proxy', component: ProxySettings, meta: { showBack: true } },
        { path: 'logs', name: 'settings-logs', component: LogsSettings, meta: { showBack: true } },
        { path: 'about', name: 'settings-about', component: AboutSettings, meta: { showBack: true } }
      ]
    }
  ]
});
