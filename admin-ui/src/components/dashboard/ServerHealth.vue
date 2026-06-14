<template>
  <div class="server-health">
    <div class="section-label">Server Health</div>

    <div class="health-grid">
      <div class="card status-card">
        <div class="card-body">
          <div class="status-left">
            <connection-pulse :status="idOnline ? 'online' : 'offline'" />
            <div class="status-info">
              <div class="status-title">ID Server</div>
              <div class="status-addr">127.0.0.1:{{ idPort }}</div>
            </div>
          </div>
          <span class="status-badge" :class="idOnline ? 'online' : 'offline'">{{ idOnline ? 'Online' : 'Offline' }}</span>
        </div>
      </div>
      <div class="card status-card">
        <div class="card-body">
          <div class="status-left">
            <connection-pulse :status="relayOnline ? 'online' : 'offline'" />
            <div class="status-info">
              <div class="status-title">Relay Server</div>
              <div class="status-addr">127.0.0.1:{{ relayPort }}</div>
            </div>
          </div>
          <span class="status-badge" :class="relayOnline ? 'online' : 'offline'">{{ relayOnline ? 'Online' : 'Offline' }}</span>
        </div>
      </div>
    </div>

    <div class="card">
      <div class="card-header">
        <span>Relay Activity</span>
        <span class="pulse-label" v-if="loading">Loading...</span>
        <span class="pulse-label" v-else>
          <span class="pulse-dot-sm"></span>
          updating every 15s
        </span>
      </div>
      <div class="card-body" v-loading="loading">
        <div class="sessions-bw-row">
          <div>
            <div class="stat-label-sm">Active Sessions</div>
            <div class="big-number">{{ activeConnections }}</div>
          </div>
          <div>
            <div class="stat-label-sm">Bandwidth</div>
            <div class="bw-list">
              <div class="bw-row">
                <div class="bw-label"><span>TOTAL</span><span>{{ totalBW }} / 100 Mb/s</span></div>
                <div class="bw-track"><div class="bw-fill bw-total" :style="{ width: bwPct(totalBW, 100) + '%' }"></div></div>
              </div>
              <div class="bw-row">
                <div class="bw-label"><span>SINGLE</span><span>{{ singleBW }} / 10 Mb/s</span></div>
                <div class="bw-track"><div class="bw-fill bw-single" :style="{ width: bwPct(singleBW, 10) + '%' }"></div></div>
              </div>
              <div class="bw-row">
                <div class="bw-label"><span>LIMIT</span><span>{{ limitSpeed }} / 50 Mb/s</span></div>
                <div class="bw-track"><div class="bw-fill bw-limit" :style="{ width: bwPct(limitSpeed, 50) + '%' }"></div></div>
              </div>
            </div>
          </div>
        </div>

        <div v-if="usage.length" class="top-connections">
          <div class="top-connections-header">
            <span class="stat-label-sm">Top Connections</span>
            <span class="top-by-label">by total traffic</span>
          </div>
          <table class="top-table">
            <thead>
              <tr><th>IP</th><th>Duration</th><th>Total</th><th>Speed</th></tr>
            </thead>
            <tbody>
              <tr v-for="row in usage" :key="row.ip">
                <td class="top-ip">{{ row.ip }}</td>
                <td>{{ fmtDuration(row.time) }}</td>
                <td class="top-total">{{ fmtTotal(row.total) }}</td>
                <td class="top-speed">{{ row.speed }} kb/s</td>
              </tr>
            </tbody>
          </table>
          <span v-if="extraCount > 0" class="top-more">+ and {{ extraCount }} more connections</span>
        </div>
        <div v-else-if="relayOnline" class="top-connections-empty">
          {{ T('NoData') }}
        </div>
      </div>
      <div class="card-footer">
        <div class="timer">
          <svg width="13" height="13" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.5"><circle cx="10" cy="10" r="8"/><path d="M10 5v5l3 3"/></svg>
          <span>{{ countdown }}s</span>
        </div>
        <div class="footer-actions">
          <router-link to="/admin/server/cmd" class="btn-link">All connections →</router-link>
          <button class="btn" @click="refreshNow">
            <svg width="12" height="12" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="2"><path d="M1 4v6h6M19 16v-6h-6"/><path d="M17.2 6.8a8 8 0 00-14.4 0M2.8 13.2a8 8 0 0014.4 0"/></svg>
            Refresh
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script>
import { defineComponent, ref, onMounted, onUnmounted } from 'vue'
import { health as fetchHealth } from '@/api/dashboard'
import { T } from '@/utils/i18n'
import ConnectionPulse from '@/components/ui/ConnectionPulse.vue'

export default defineComponent({
  name: 'ServerHealth',
  components: { ConnectionPulse },
  setup () {
    const idOnline = ref(false)
    const relayOnline = ref(false)
    const activeConnections = ref(0)
    const totalBW = ref(0)
    const singleBW = ref(0)
    const limitSpeed = ref(0)
    const usage = ref([])
    const extraCount = ref(0)
    const loading = ref(true)
    const idPort = ref('21115')
    const relayPort = ref('21117')
    const countdown = ref(0)

    let interval = null
    let countdownInterval = null

    const load = async () => {
      try {
        const res = await fetchHealth()
        const d = res.data
        idOnline.value = !!d.id_server?.online
        relayOnline.value = !!d.relay_server?.online
        activeConnections.value = d.active_connections || 0
        totalBW.value = d.total_bandwidth || 0
        singleBW.value = d.single_bandwidth || 0
        limitSpeed.value = d.limit_speed || 0
        usage.value = d.usage || []
        extraCount.value = Math.max(0, activeConnections.value - (d.usage || []).length)
      } catch (e) {
        console.error('Failed to load server health', e)
      } finally {
        loading.value = false
      }
    }

    const refreshNow = () => {
      countdown.value = 15
      load()
    }

    const bwPct = (val, max) => Math.min(100, Math.round((val / max) * 100))

    const fmtDuration = (seconds) => {
      if (!seconds) return '-'
      const h = Math.floor(seconds / 3600)
      const m = Math.floor((seconds % 3600) / 60)
      if (h > 0) return `${h}h ${m}m`
      return `${m}m`
    }

    const fmtTotal = (mb) => {
      if (!mb) return '-'
      if (mb >= 1024) return (mb / 1024).toFixed(1) + ' GB'
      return Math.round(mb) + ' MB'
    }

    onMounted(() => {
      load()
      countdown.value = 15
      interval = setInterval(load, 15000)
      countdownInterval = setInterval(() => {
        if (countdown.value > 0) countdown.value--
        else countdown.value = 15
      }, 1000)
    })

    onUnmounted(() => {
      if (interval) clearInterval(interval)
      if (countdownInterval) clearInterval(countdownInterval)
    })

    return {
      idOnline, relayOnline, activeConnections,
      totalBW, singleBW, limitSpeed,
      usage, extraCount, loading,
      idPort, relayPort,
      countdown, refreshNow,
      bwPct, fmtDuration, fmtTotal, T,
    }
  },
})
</script>

<style scoped lang="scss">
.server-health {
  margin-bottom: 20px;
}

.section-label {
  font-family: var(--font-mono);
  font-size: 11px;
  font-weight: 600;
  letter-spacing: .06em;
  text-transform: uppercase;
  color: var(--color-muted);
  margin-bottom: 12px;
}

.health-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 16px;
  margin-bottom: 16px;
}

.card {
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
  box-shadow: var(--shadow-card);
  overflow: hidden;
}

.card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 14px 18px;
  border-bottom: 1px solid var(--color-border);
  font-size: 13px;
  font-weight: 600;
}

.card-body {
  padding: 18px;
}

.card-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 10px 18px;
  border-top: 1px solid var(--color-border);
  background: var(--color-bg);
  font-size: 12px;
  color: var(--color-muted);
}

.footer-actions {
  display: flex;
  gap: 8px;
  align-items: center;
}

.timer {
  display: flex;
  align-items: center;
  gap: 5px;
}

.status-card .card-body {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 18px 20px;
  gap: 12px;
  flex: 1;
}

.status-left {
  display: flex;
  align-items: center;
  gap: 14px;
}

.status-title {
  font-size: 12px;
  color: var(--color-muted);
  font-weight: 500;
  letter-spacing: .02em;
  margin-bottom: 2px;
}

.status-addr {
  font-family: var(--font-mono);
  font-size: 12px;
  color: var(--color-text);
  font-weight: 500;
}

.status-badge {
  font-size: 11px;
  font-weight: 700;
  letter-spacing: .05em;
  text-transform: uppercase;
  padding: 4px 10px;
  border-radius: 999px;
  flex-shrink: 0;
}

.status-badge.online {
  background: var(--color-success-soft);
  color: var(--color-success);
}

.status-badge.offline {
  background: var(--color-danger-soft);
  color: var(--color-danger);
}

.sessions-bw-row {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 16px;
}

.stat-label-sm {
  font-size: 12px;
  color: var(--color-muted);
  font-weight: 500;
  letter-spacing: .02em;
}

.big-number {
  font-size: 36px;
  font-weight: 700;
  line-height: 1;
  letter-spacing: -.04em;
  margin-top: 4px;
  color: var(--color-primary);
}

.bw-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.bw-label {
  display: flex;
  justify-content: space-between;
  font-size: 12px;
  margin-bottom: 3px;
}

.bw-label span:first-child {
  font-weight: 500;
  font-family: var(--font-mono);
  font-size: 11px;
}

.bw-label span:last-child {
  color: var(--color-muted);
}

.bw-track {
  height: 5px;
  border-radius: 999px;
  background: var(--color-surface-2);
  overflow: hidden;
}

.bw-fill {
  height: 100%;
  border-radius: 999px;
  transition: width .6s ease;
}

.bw-total { background: var(--color-primary); }
.bw-single { background: var(--color-warning); }
.bw-limit { background: var(--color-success); }

.top-connections {
  margin-top: 16px;
  padding-top: 14px;
  border-top: 1px solid var(--color-border);
}

.top-connections-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 4px;
}

.top-by-label {
  font-size: 11px;
  color: var(--color-muted);
}

.top-connections-empty {
  margin-top: 16px;
  padding-top: 14px;
  border-top: 1px solid var(--color-border);
  text-align: center;
  color: var(--color-muted);
  font-size: 13px;
}

.top-table {
  width: 100%;
  border-collapse: collapse;
  margin-top: 4px;
}

.top-table th {
  text-align: left;
  font-size: 11px;
  font-weight: 600;
  color: var(--color-muted);
  text-transform: uppercase;
  letter-spacing: .04em;
  padding: 0 10px 7px 0;
  border-bottom: 1px solid var(--color-border);
}

.top-table th:last-child {
  text-align: right;
  padding-right: 0;
}

.top-table td {
  padding: 6px 10px 6px 0;
  border-bottom: 1px solid var(--color-border);
  font-size: 13px;
  vertical-align: middle;
}

.top-table td:last-child {
  text-align: right;
  padding-right: 0;
}

.top-table tr:last-child td {
  border-bottom: none;
}

.top-ip {
  font-family: var(--font-mono);
  font-size: 12px;
  font-weight: 500;
}

.top-total {
  font-weight: 600;
}

.top-speed {
  color: var(--color-muted);
  font-size: 12px;
}

.top-more {
  display: block;
  font-size: 12px;
  color: var(--color-muted);
  padding: 8px 0 0;
  text-align: center;
  border-top: 1px solid var(--color-border);
  margin-top: 6px;
}

.pulse-label {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 11px;
  color: var(--color-muted);
}

.pulse-dot-sm {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: var(--color-success);
  box-shadow: 0 0 0 2px var(--color-success-soft);
}

.btn {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  padding: 5px 12px;
  border: 1px solid var(--color-border);
  border-radius: 999px;
  background: var(--color-surface);
  color: var(--color-text);
  font-size: 12px;
  font-weight: 500;
  cursor: pointer;
  transition: .15s;
}

.btn:hover {
  border-color: var(--color-primary);
  color: var(--color-primary);
}

.btn-link {
  color: var(--color-primary);
  font-weight: 500;
  text-decoration: none;
  font-size: 12px;
}

.btn-link:hover {
  text-decoration: underline;
}

@media (max-width: 1024px) {
  .health-grid {
    grid-template-columns: 1fr;
  }

  .sessions-bw-row {
    grid-template-columns: 1fr;
  }
}
</style>
