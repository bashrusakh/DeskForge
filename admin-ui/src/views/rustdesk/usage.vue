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
  const getList = async () => {
    const res = await sendCmd({ cmd: form.get_cmd, target: RELAY_TARGET }).catch(_ => false)
    if (res) {
      form.list = res.data.split('\n').filter(i => i).map(i => i.split(" "))
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
