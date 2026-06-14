<template>
  <div class="dashboard">
    <section class="quick-connect-panel">
      <div class="quick-connect-copy">
        <div class="panel-kicker">
          <connection-pulse status="online" />
          Connection Pulse
        </div>
        <h1>Connect to a device</h1>
        <p>Enter a RustDesk ID to open the native client, launch the web client, or jump to the device list.</p>
      </div>
      <div class="quick-connect-form">
        <el-input v-model="quickId" clearable placeholder="RustDesk ID" class="quick-id-input" @keyup.enter="connectNative">
          <template #prefix>
            <el-icon><Connection /></el-icon>
          </template>
        </el-input>
        <el-button type="primary" @click="connectNative" :disabled="!quickId">Connect</el-button>
        <el-button @click="openWebClient" :disabled="!quickId || !webClientEnabled">Web client</el-button>
        <el-button text @click="$router.push('/admin/devices')">{{ T('AllDevices') }}</el-button>
      </div>
    </section>

    <server-health />

    <div class="stats-cards">
      <el-card shadow="hover" class="stat-card">
        <div class="stat-inner">
          <div class="stat-icon users"><el-icon><User /></el-icon></div>
          <div class="stat-body">
            <div class="stat-num">{{ stats.total_users }}</div>
            <div class="stat-label">{{ T('TotalUsers') }}</div>
          </div>
        </div>
      </el-card>
      <el-card shadow="hover" class="stat-card">
        <div class="stat-inner">
          <div class="stat-icon devices"><el-icon><Monitor /></el-icon></div>
          <div class="stat-body">
            <div class="stat-num">{{ stats.total_peers }}</div>
            <div class="stat-label">{{ T('TotalPeers') }}</div>
          </div>
        </div>
      </el-card>
      <el-card shadow="hover" class="stat-card">
        <div class="stat-inner">
          <div class="stat-icon online"><el-icon><Connection /></el-icon></div>
          <div class="stat-body">
            <div class="stat-num">{{ stats.online_peers }}</div>
            <div class="stat-label stat-label-pulse"><connection-pulse status="online" />{{ T('OnlinePeers') }}</div>
          </div>
        </div>
      </el-card>
      <el-card shadow="hover" class="stat-card">
        <div class="stat-inner">
          <div class="stat-icon groups"><el-icon><ChatRound /></el-icon></div>
          <div class="stat-body">
            <div class="stat-num">{{ stats.total_groups }}</div>
            <div class="stat-label">{{ T('TotalGroups') }}</div>
          </div>
        </div>
      </el-card>
      <el-card shadow="hover" class="stat-card">
        <div class="stat-inner">
          <div class="stat-icon logins"><el-icon><List /></el-icon></div>
          <div class="stat-body">
            <div class="stat-num">{{ stats.total_logins }}</div>
            <div class="stat-label">{{ T('TotalLogins') }}</div>
          </div>
        </div>
      </el-card>
      <el-card shadow="hover" class="stat-card">
        <div class="stat-inner">
          <div class="stat-icon recent"><el-icon><Timer /></el-icon></div>
          <div class="stat-body">
            <div class="stat-num">{{ stats.recent_logins }}</div>
            <div class="stat-label">{{ T('RecentLogins') }}</div>
          </div>
        </div>
      </el-card>
    </div>

    <el-row :gutter="20" class="dashboard-grid">
      <el-col :xs="24" :lg="12">
        <el-card shadow="hover">
          <template #header>
            <span class="card-title">{{ T('QuickActions') }}</span>
          </template>
          <div class="quick-actions">
            <el-button type="primary" @click="$router.push('/admin/devices')">{{ T('AllDevices') }}</el-button>
            <el-button type="success" @click="$router.push('/admin/users')">{{ T('Users') }}</el-button>
            <el-button type="warning" @click="$router.push('/admin/monitoring/login-logs')">{{ T('LoginHistory') }}</el-button>
            <el-button @click="$router.push('/admin/server/config')">{{ T('ServerConfig') }}</el-button>
          </div>
        </el-card>
      </el-col>
      <el-col :xs="24" :lg="12">
        <el-card shadow="hover">
          <template #header>
            <span class="card-title">{{ T('RecentActivity') }}</span>
          </template>
          <div v-if="recentLogs.length" class="activity-list">
            <div v-for="log in recentLogs" :key="log.id" class="activity-item">
              <el-tag size="small" :type="log.type === 'account' ? '' : 'success'">{{ log.type }}</el-tag>
              <span class="activity-user">{{ log.client }}</span>
              <span class="activity-time">{{ log.created_at }}</span>
            </div>
          </div>
          <el-empty v-else :description="T('NoData')" />
        </el-card>
      </el-col>
    </el-row>
  </div>
</template>

<script>
import { computed, defineComponent, onMounted, ref } from 'vue'
import { stats as fetchStats } from '@/api/dashboard'
import { list as fetchLoginLogs } from '@/api/login_log'
import { User, Monitor, Connection, ChatRound, List, Timer } from '@element-plus/icons-vue'
import { T } from '@/utils/i18n'
import { useAppStore } from '@/store/app'
import { connectByClient } from '@/utils/peer'
import ConnectionPulse from '@/components/ui/ConnectionPulse.vue'
import ServerHealth from '@/components/dashboard/ServerHealth.vue'

export default defineComponent({
  name: 'Home',
  components: { User, Monitor, Connection, ChatRound, List, Timer, ConnectionPulse, ServerHealth },
  setup () {
    const appStore = useAppStore()
    const stats = ref({
      total_users: 0,
      total_peers: 0,
      online_peers: 0,
      total_groups: 0,
      total_logins: 0,
      recent_logins: 0,
    })
    const recentLogs = ref([])
    const loading = ref(true)
    const quickId = ref('')
    const webClientEnabled = computed(() => Number(appStore.setting.appConfig.web_client) === 1)

    const loadData = async () => {
      try {
        const statRes = await fetchStats()
        stats.value = statRes.data
        const logRes = await fetchLoginLogs({ page: 1, page_size: 10 })
        recentLogs.value = logRes.data.list || []
      } catch (e) {
        console.error('Failed to load dashboard', e)
      } finally {
        loading.value = false
      }
    }

    const connectNative = () => {
      const id = quickId.value.trim()
      if (id) {
        connectByClient(id)
      }
    }

    const openWebClient = () => {
      const id = quickId.value.trim()
      if (id && webClientEnabled.value) {
        const apiServer = appStore.setting.rustdeskConfig.api_server || window.location.origin
        window.open(`${apiServer}/webclient2/#/${id}`)
      }
    }

    onMounted(loadData)

    return {
      stats,
      recentLogs,
      loading,
      quickId,
      webClientEnabled,
      connectNative,
      openWebClient,
      T,
    }
  },
})
</script>

<style scoped lang="scss">
.dashboard {
  .quick-connect-panel {
    position: relative;
    display: grid;
    grid-template-columns: minmax(0, 1fr) minmax(360px, 0.9fr);
    gap: 24px;
    align-items: end;
    margin-bottom: 20px;
    padding: 28px;
    overflow: hidden;
    border: 1px solid var(--color-border);
    border-radius: 24px;
    background:
      linear-gradient(135deg, color-mix(in srgb, var(--color-primary) 12%, transparent), transparent 46%),
      var(--color-surface);
    box-shadow: var(--shadow-card);

    &::after {
      content: '';
      position: absolute;
      right: -90px;
      top: -120px;
      width: 260px;
      height: 260px;
      border: 1px solid color-mix(in srgb, var(--color-primary) 28%, transparent);
      border-radius: 999px;
      box-shadow: 0 0 0 34px color-mix(in srgb, var(--color-primary) 7%, transparent);
      pointer-events: none;
    }
  }

  .quick-connect-copy,
  .quick-connect-form {
    position: relative;
    z-index: 1;
  }

  .panel-kicker {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-bottom: 10px;
    color: var(--color-muted);
    font-family: var(--font-mono);
    font-size: 12px;
    font-weight: 600;
    letter-spacing: 0.04em;
    text-transform: uppercase;
  }

  h1 {
    margin: 0;
    color: var(--color-text);
    font-size: clamp(28px, 4vw, 42px);
    line-height: 1;
    letter-spacing: -0.04em;
  }

  p {
    max-width: 560px;
    margin: 12px 0 0;
    color: var(--color-muted);
    font-size: 15px;
    line-height: 1.6;
  }

  .quick-connect-form {
    display: flex;
    flex-wrap: wrap;
    gap: 10px;
    justify-content: flex-end;

    .quick-id-input {
      flex: 1 1 190px;
      min-width: 190px;

      :deep(.el-input__wrapper) {
        min-height: 42px;
        background: var(--color-bg);
        font-family: var(--font-mono);
      }
    }
  }

  .stats-cards {
    display: flex;
    flex-wrap: wrap;
    gap: 16px;
    margin-bottom: 20px;

    .stat-card {
      flex: 1;
      min-width: 180px;
      border-radius: var(--radius-lg);

      .stat-inner {
        display: flex;
        align-items: center;
        gap: 16px;

        .stat-icon {
          width: 48px;
          height: 48px;
          border-radius: 12px;
          display: flex;
          align-items: center;
          justify-content: center;
          font-size: 24px;

          &.users { background: var(--color-primary-soft); color: var(--color-primary); }
          &.devices { background: var(--color-success-soft); color: var(--color-success); }
          &.online { background: var(--color-success-soft); color: var(--color-success); }
          &.groups { background: var(--color-danger-soft); color: var(--color-danger); }
          &.logins { background: var(--color-surface-2); color: var(--color-muted); }
          &.recent { background: var(--color-primary-soft); color: var(--color-primary); }
        }

        .stat-body {
          .stat-num {
            color: var(--color-text);
            font-size: 32px;
            font-weight: 700;
            line-height: 1.2;
          }
          .stat-label {
            display: flex;
            align-items: center;
            gap: 7px;
            font-size: 13px;
            color: var(--color-muted);
          }
        }
      }
    }
  }

  .dashboard-grid {
    row-gap: 20px;
  }

  .quick-actions {
    display: flex;
    flex-wrap: wrap;
    gap: 10px;
  }

  .activity-list {
    .activity-item {
      display: flex;
      align-items: center;
      gap: 8px;
      padding: 6px 0;
      border-bottom: 1px solid var(--color-border);

      .activity-user {
        flex: 1;
        font-size: 13px;
      }
      .activity-time {
        color: var(--color-muted);
        font-size: 12px;
      }
    }
  }

  .card-title {
    font-weight: 600;
  }
}

@media (max-width: 1024px) {
  .dashboard .quick-connect-panel {
    grid-template-columns: 1fr;
  }

  .dashboard .quick-connect-form {
    justify-content: flex-start;
  }
}

@media (max-width: 640px) {
  .dashboard .quick-connect-panel {
    padding: 20px;
  }

  .dashboard .quick-connect-form .el-button,
  .dashboard .quick-connect-form .quick-id-input {
    width: 100%;
  }
}
</style>
