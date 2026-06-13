<template>
  <div class="dashboard">
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
            <div class="stat-label">{{ T('OnlinePeers') }}</div>
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

    <el-row :gutter="20">
      <el-col :span="12">
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
      <el-col :span="12">
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
import { defineComponent, ref, onMounted } from 'vue'
import { stats as fetchStats } from '@/api/dashboard'
import { list as fetchLoginLogs } from '@/api/login_log'
import { User, Monitor, Connection, ChatRound, List, Timer } from '@element-plus/icons-vue'
import { T } from '@/utils/i18n'

export default defineComponent({
  name: 'Home',
  components: { User, Monitor, Connection, ChatRound, List, Timer },
  setup () {
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

    onMounted(loadData)

    return {
      stats,
      recentLogs,
      loading,
      T,
    }
  },
})
</script>

<style scoped lang="scss">
.dashboard {
  .stats-cards {
    display: flex;
    flex-wrap: wrap;
    gap: 16px;
    margin-bottom: 20px;

    .stat-card {
      flex: 1;
      min-width: 180px;

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

          &.users { background: #ecf5ff; color: #409eff; }
          &.devices { background: #f0f9eb; color: #67c23a; }
          &.online { background: #fdf6ec; color: #e6a23c; }
          &.groups { background: #fef0f0; color: #f56c6c; }
          &.logins { background: #f5f7fa; color: #909399; }
          &.recent { background: #edf5ff; color: #337ecc; }
        }

        .stat-body {
          .stat-num {
            font-size: 28px;
            font-weight: bold;
            line-height: 1.2;
          }
          .stat-label {
            font-size: 13px;
            color: #909399;
          }
        }
      }
    }
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
      border-bottom: 1px solid #f0f0f0;

      .activity-user {
        flex: 1;
        font-size: 13px;
      }
      .activity-time {
        font-size: 12px;
        color: #909399;
      }
    }
  }

  .card-title {
    font-weight: 600;
  }
}
</style>
