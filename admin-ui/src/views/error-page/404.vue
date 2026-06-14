<template>
  <main class="not-found-page">
    <empty-state
        title="Page not found"
        description="The route you opened does not exist or is no longer available in this admin console."
    >
      <template #icon>
        <span class="not-found-page__code">404</span>
      </template>
      <template #actions>
        <el-button type="primary" @click="goDashboard">Back to dashboard</el-button>
      </template>
    </empty-state>
  </main>
</template>

<script setup>
  import { useRouter } from 'vue-router'
  import EmptyState from '@/components/ui/EmptyState.vue'

  defineOptions({ name: 'NotFound' })

  const router = useRouter()
  const goDashboard = () => {
    router.push('/dashboard')
  }
</script>

<style scoped lang="scss">
.not-found-page {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px;
  background:
    radial-gradient(circle at top left, color-mix(in srgb, var(--color-primary) 16%, transparent), transparent 32%),
    var(--color-bg);
}

.not-found-page__code {
  font-family: var(--font-mono);
  font-size: 18px;
  font-weight: 800;
  letter-spacing: -0.04em;
}
</style>
