<template>
  <el-card class="simple-card" shadow="never">
    <template #header>
      <div class="card-header">
        <span>USAGE</span>
      </div>
    </template>
    <el-form :disabled="!canSend">
      <el-form-item>
          <data-table
              :data="form.list"
              row-key="0"
              :columns="[
                { prop: '0', label: 'IP' },
                { prop: '1', label: 'TIME' },
                { prop: '2', label: 'TOTAL' },
                { prop: '3', label: 'HIGHEST' },
                { prop: '4', label: 'AVG' },
                { prop: '5', label: 'SPEED' }
              ]"
              size="small"
          />
      </el-form-item>
      <el-form-item>
        <el-button @click="getList">{{ T('Refresh') }}</el-button>
      </el-form-item>
    </el-form>
  </el-card>
</template>
<script setup>

  import { T } from '@/utils/i18n'
  import { reactive, watch } from 'vue'
  import { sendCmd } from '@/api/rustdesk'
  import { RELAY_TARGET } from '@/views/rustdesk/options'
  import DataTable from '@/components/ui/DataTable.vue'

  const props = defineProps({
    canSend: Boolean,
  })

  const form = reactive({
    get_cmd: 'u',
    list: [],
    target: RELAY_TARGET,
  })
  // AU-M-014: устойчивый парсинг ответа usage. Раньше res.data.split('\n')...
  // split(" ") падал, если data не строка, и ломался на CRLF / повторных пробелах.
  const parseUsage = (raw) => {
    if (typeof raw !== 'string') {
      return []
    }
    return raw
      .split(/\r?\n/)
      .map(line => line.trim())
      .filter(line => line.length > 0)
      .map(line => line.split(/\s+/))
  }
  const getList = async () => {
    const res = await sendCmd({ cmd: form.get_cmd, target: RELAY_TARGET }).catch(_ => false)
    if (res) {
      form.list = parseUsage(res.data)
    }
  }
  watch(() => props.canSend, (v) => {
    if (v) {
      getList()
    }
  })


</script>
<style scoped lang="scss">
.simple-card{
  width: 500px;
}
</style>
