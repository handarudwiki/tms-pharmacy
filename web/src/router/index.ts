import { createRouter, createWebHistory } from 'vue-router'
import { useAuth } from '@/stores/auth'

const page = (name: string) => () => import(`../views/${name}.vue`)

export const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/login', name: 'login', component: page('LoginView'), meta: { public: true } },
    {
      path: '/',
      component: () => import('../layouts/AppLayout.vue'),
      children: [
        { path: '', name: 'dashboard', component: page('DashboardView') },
        { path: 'shipments', name: 'shipments', component: page('ShipmentsView') },
        { path: 'planning', name: 'planning', component: page('PlanningView') },
        { path: 'trips', name: 'trips', component: page('TripsView') },
        { path: 'tracking', name: 'tracking', component: page('TrackingView') },
        { path: 'driver', name: 'driver', component: page('DriverView') },
        { path: 'fleet', name: 'fleet', component: page('FleetView') },
        { path: 'freight', name: 'freight', component: page('FreightView') },
        { path: 'exceptions', name: 'exceptions', component: page('ExceptionsView') },
        { path: 'analytics', name: 'analytics', component: page('AnalyticsView') },
        { path: 'integrations', name: 'integrations', component: page('IntegrationsView') },
        { path: 'users', name: 'users', component: page('UsersView') },
        { path: 'audit', name: 'audit', component: page('AuditView') },
      ],
    },
    { path: '/:pathMatch(.*)*', redirect: '/' },
  ],
})

router.beforeEach((to) => {
  const auth = useAuth()
  if (!to.meta.public && !auth.isAuthenticated) return { name: 'login', query: { next: to.fullPath } }
  if (to.name === 'login' && auth.isAuthenticated) return { name: auth.isDriverOnly ? 'driver' : 'dashboard' }
  if (to.name === 'dashboard' && auth.isDriverOnly) return { name: 'driver' }
})
