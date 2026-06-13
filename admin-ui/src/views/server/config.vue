<template>
  <div class="server-config">
    <page-header
        :title="T('ServerConfig')"
        subtitle="Read-only server endpoints and feature flags used by the admin UI, API, and web client."
        eyebrow="Server"
        pulse="online"
    />
    <el-row :gutter="20">
      <el-col :xs="24" :lg="12">
        <page-section :title="T('ServerConfig')" subtitle="Connection endpoints and public key material.">
          <el-descriptions :column="1" border v-loading="loading">
            <el-descriptions-item :label="T('Host')">
              <code>{{ cfg.id_server || '-' }}</code>
            </el-descriptions-item>
            <el-descriptions-item label="Key">
              <code>{{ cfg.key ? cfg.key.substring(0, 20) + '...' : '-' }}</code>
            </el-descriptions-item>
            <el-descriptions-item :label="T('RelayServer')">
              <code>{{ cfg.relay_server || '-' }}</code>
            </el-descriptions-item>
            <el-descriptions-item :label="T('ApiServer')">
              <code>{{ cfg.api_server || '-' }}</code>
            </el-descriptions-item>
            <el-descriptions-item label="WebSocket">
              <code>{{ cfg.ws_host || '-' }}</code>
            </el-descriptions-item>
          </el-descriptions>
        </page-section>
      </el-col>
      <el-col :xs="24" :lg="12">
        <page-section :title="T('System')" subtitle="Runtime features exposed by the server.">
          <el-descriptions :column="1" border v-loading="loading">
            <el-descriptions-item label="Web Client">
              <el-tag :type="cfg.web_client === 1 ? 'success' : 'info'" size="small">{{ cfg.web_client === 1 ? T('Available') : T('NotAvailable') }}</el-tag>
            </el-descriptions-item>
            <el-descriptions-item :label="T('Register')">
              <el-tag :type="cfg.register ? 'success' : 'info'" size="small">{{ cfg.register ? T('Available') : T('NotAvailable') }}</el-tag>
            </el-descriptions-item>
            <el-descriptions-item label="Swagger">
              <el-tag :type="cfg.show_swagger === 1 ? 'success' : 'info'" size="small">{{ cfg.show_swagger === 1 ? T('Available') : T('NotAvailable') }}</el-tag>
            </el-descriptions-item>
            <el-descriptions-item label="Personal API">
              <el-tag :type="cfg.personal === 1 ? 'success' : 'info'" size="small">{{ cfg.personal === 1 ? T('Available') : T('NotAvailable') }}</el-tag>
            </el-descriptions-item>
            <el-descriptions-item label="Language">
              <span>{{ cfg.lang || 'en' }}</span>
            </el-descriptions-item>
            <el-descriptions-item label="Token Expiry">
              <span>{{ cfg.token_expire || '-' }}</span>
            </el-descriptions-item>
          </el-descriptions>
        </page-section>
      </el-col>
    </el-row>
  </div>
</template>

<script>
import { defineComponent, ref, onMounted } from 'vue'
import { all as fetchAllConfig } from '@/api/config'
import { T } from '@/utils/i18n'
import PageHeader from '@/components/ui/PageHeader.vue'
import PageSection from '@/components/ui/PageSection.vue'

export default defineComponent({
  name: 'ServerConfig',
  components: { PageHeader, PageSection },
  setup () {
    const cfg = ref({})
    const loading = ref(true)

    const loadData = async () => {
      try {
        const res = await fetchAllConfig()
        cfg.value = res.data || {}
      } catch (e) {
        console.error(e)
      } finally {
        loading.value = false
      }
    }

    onMounted(loadData)

    return { cfg, loading, T }
  },
})
</script>

<style scoped lang="scss">
.server-config {
  code {
    padding: 3px 6px;
    border-radius: 8px;
    background: var(--color-code-bg);
    color: var(--color-text);
    font-family: var(--font-mono);
    font-size: 12px;
  }
}
</style>
