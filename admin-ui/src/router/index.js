import { createRouter, createWebHashHistory } from 'vue-router'

const constantRoutes = [
  {
    path: '/login',
    name: 'Login',
    meta: { title: 'Login' },
    component: () => import('@/views/login/login.vue'),
  },
  {
    path: '/register',
    name: 'Register',
    meta: { title: 'Register' },
    component: () => import('@/views/register/index.vue'),
  },
  {
    path: '/404',
    component: () => import('@/views/error-page/404.vue'),
    hidden: true,
  },
  {
    path: '/oauth/:code',
    meta: { title: 'OauthLogin' },
    component: () => import('@/views/oauth/login.vue'),
    hidden: true,
  },
  {
    path: '/oauth/bind/:code',
    meta: { title: 'OauthBind' },
    component: () => import('@/views/oauth/bind.vue'),
    hidden: true,
  },
]
export const asyncRoutes = [
  {
    path: '/',
    redirect: '/dashboard',
  },
  // === Dashboard ===
  {
    path: '/dashboard',
    name: 'Dashboard',
    meta: { title: 'Dashboard', icon: 'Odometer' },
    component: () => import('@/layout/index.vue'),
    children: [
      {
        path: '',
        name: 'DashboardHome',
        meta: { title: 'Dashboard', icon: 'Odometer' },
        component: () => import('@/views/index/index.vue'),
      },
    ],
  },
  // === Devices (admin) ===
  {
    path: '/admin/devices',
    name: 'AdminDevices',
    meta: { title: 'Devices', icon: 'Monitor' },
    component: () => import('@/layout/index.vue'),
    children: [
      {
        path: '',
        name: 'Peer',
        meta: { title: 'AllDevices', icon: 'Monitor' },
        component: () => import('@/views/peer/index.vue'),
      },
      {
        path: 'groups',
        name: 'DeviceGroup',
        meta: { title: 'DeviceGroupManage', icon: 'ChatRound' },
        component: () => import('@/views/group/deviceGroupList.vue'),
      },
    ],
  },
  // === Users (admin) ===
  {
    path: '/admin/users',
    name: 'AdminUsers',
    meta: { title: 'Users', icon: 'User' },
    component: () => import('@/layout/index.vue'),
    children: [
      {
        path: '',
        name: 'UserList',
        meta: { title: 'Users', icon: 'User' },
        component: () => import('@/views/user/index.vue'),
      },
      {
        path: 'add',
        name: 'UserAdd',
        meta: { title: 'UserAdd', hide: true },
        component: () => import('@/views/user/edit.vue'),
      },
      {
        path: 'edit/:id',
        name: 'UserEdit',
        meta: { title: 'UserEdit', hide: true },
        component: () => import('@/views/user/edit.vue'),
      },
    ],
  },
  // === Groups (admin) ===
  {
    path: '/admin/groups',
    name: 'AdminGroups',
    meta: { title: 'Groups', icon: 'ChatRound' },
    component: () => import('@/layout/index.vue'),
    children: [
      {
        path: '',
        name: 'UserGroup',
        meta: { title: 'GroupManage', icon: 'ChatRound' },
        component: () => import('@/views/group/index.vue'),
      },
    ],
  },
  // === Address Book (admin) ===
  {
    path: '/admin/address-book',
    name: 'AdminAddressBook',
    meta: { title: 'AddressBook', icon: 'Notebook' },
    component: () => import('@/layout/index.vue'),
    children: [
      {
        path: 'collections',
        name: 'UserAddressBookName',
        meta: { title: 'Collections', icon: 'Collection' },
        component: () => import('@/views/address_book/collection.vue'),
      },
      {
        path: 'books',
        name: 'UserAddressBook',
        meta: { title: 'Contacts', icon: 'Notebook' },
        component: () => import('@/views/address_book/index.vue'),
      },
      {
        path: 'tags',
        name: 'UserTag',
        meta: { title: 'TagsManage', icon: 'CollectionTag' },
        component: () => import('@/views/tag/index.vue'),
      },
    ],
  },
  // === Security (admin) ===
  {
    path: '/admin/security',
    name: 'AdminSecurity',
    meta: { title: 'Security', icon: 'Lock' },
    component: () => import('@/layout/index.vue'),
    children: [
      {
        path: 'oauth',
        name: 'Oauth',
        meta: { title: 'SSOProviders', icon: 'Link' },
        component: () => import('@/views/oauth/index.vue'),
      },
      {
        path: 'tokens',
        name: 'UserToken',
        meta: { title: 'APITokens', icon: 'Ticket' },
        component: () => import('@/views/user/token.vue'),
      },
    ],
  },
  // === Monitoring (admin) ===
  {
    path: '/admin/monitoring',
    name: 'AdminMonitoring',
    meta: { title: 'Monitoring', icon: 'DataAnalysis' },
    component: () => import('@/layout/index.vue'),
    children: [
      {
        path: 'login-logs',
        name: 'LoginLog',
        meta: { title: 'LoginHistory', icon: 'List' },
        component: () => import('@/views/login/log.vue'),
      },
      {
        path: 'connections',
        name: 'AuditConn',
        meta: { title: 'ConnectionHistory', icon: 'Tickets' },
        component: () => import('@/views/audit/connList.vue'),
      },
      {
        path: 'file-transfers',
        name: 'AuditFile',
        meta: { title: 'FileTransferHistory', icon: 'Files' },
        component: () => import('@/views/audit/fileList.vue'),
      },
      {
        path: 'shares',
        name: 'ShareRecord',
        meta: { title: 'SharedSessions', icon: 'Share' },
        component: () => import('@/views/share_record/index.vue'),
      },
    ],
  },
  // === Custom Client (admin) (new) ===
  {
    path: '/admin/custom-client',
    name: 'AdminCustomClient',
    meta: { title: 'ClientBuilder', icon: 'Tools' },
    component: () => import('@/layout/index.vue'),
    children: [
      {
        path: '',
        name: 'CustomClientBuilds',
        meta: { title: 'ClientBuilder', icon: 'Tools' },
        component: () => import('@/views/custom-client/index.vue'),
      },
    ],
  },
  // === Server (admin) ===
  {
    path: '/admin/server',
    name: 'AdminServer',
    meta: { title: 'Server', icon: 'Setting' },
    component: () => import('@/layout/index.vue'),
    children: [
      {
        path: 'cmd',
        name: 'ServerCmd',
        meta: { title: 'ServerCommands', icon: 'Tools' },
        component: () => import('@/views/rustdesk/control.vue'),
      },
      {
        path: 'config',
        name: 'ServerConfig',
        meta: { title: 'ServerConfig', icon: 'Setting' },
        component: () => import('@/views/server/config.vue'),
      },
      {
        path: 'github-build',
        name: 'GithubBuildSettings',
        meta: { title: 'GithubBuildSettings', icon: 'Connection' },
        component: () => import('@/views/server/github-build.vue'),
      },
    ],
  },
  // === My Profile (user self-service) ===
  {
    path: '/my',
    name: 'MyProfile',
    redirect: '/my/info',
    meta: { title: 'MyProfile', icon: 'UserFilled' },
    component: () => import('@/layout/index.vue'),
    children: [
      {
        path: '/my/info',
        name: 'MyInfo',
        meta: { title: 'AccountInfo', icon: 'User' },
        component: () => import('@/views/my/info.vue'),
      },
      {
        path: '/my/devices',
        name: 'MyPeer',
        meta: { title: 'MyDevices', icon: 'Monitor' },
        component: () => import('@/views/my/peer/index.vue'),
      },
      {
        path: '/my/address-book-collections',
        name: 'MyAddressBookCollection',
        meta: { title: 'MyCollections', icon: 'Collection' },
        component: () => import('@/views/my/address_book/collection.vue'),
      },
      {
        path: '/my/address-books',
        name: 'MyAddressBookList',
        meta: { title: 'MyContacts', icon: 'Notebook' },
        component: () => import('@/views/my/address_book/index.vue'),
      },
      {
        path: '/my/tags',
        name: 'MyTagList',
        meta: { title: 'MyTags', icon: 'CollectionTag' },
        component: () => import('@/views/my/tag/index.vue'),
      },
      {
        path: '/my/shares',
        name: 'MyShareRecordList',
        meta: { title: 'MySharedSessions', icon: 'Share' },
        component: () => import('@/views/my/share_record/index.vue'),
      },
      {
        path: '/my/login-logs',
        name: 'MyLoginLog',
        meta: { title: 'MyLoginHistory', icon: 'List' },
        component: () => import('@/views/my/login_log/index.vue'),
      },
    ],
  },
]
export const lastRoutes = [
  { path: '/:catchAll(.*)', redirect: '/404', meta: { hide: true } },
]

export const router = createRouter({
  history: createWebHashHistory(),
  routes: constantRoutes,
})
