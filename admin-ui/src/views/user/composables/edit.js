import { ref, onMounted, reactive, watch } from 'vue'
import { create, detail, update, remove } from '@/api/user'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useRouter } from 'vue-router'
import { list as groups } from '@/api/group'
import { T } from '@/utils/i18n'
import { ENABLE_STATUS, DISABLE_STATUS } from '@/utils/common_options'

const defaultForm = () => ({
  username: '', email: '', nickname: '', remark: '',
  group_id: 0, is_admin: false, status: 1,
})

export function useGetDetail (id) {
  let item = ref({})  //保留原始值
  let form = ref(defaultForm())
  const groupsList = ref([])
  const getDetail = async (id) => {
    const res = await detail(id)
    item.value = { ...res.data }
    form.value = { ...res.data, is_admin: !!res.data.is_admin, status: res.data.status === ENABLE_STATUS ? ENABLE_STATUS : DISABLE_STATUS }
  }
  if (id > 0) {
    onMounted(_ => {getDetail(id)})
  }

  const getGroups = async () => {
    const res = await groups({ page_size: 9999 }).catch(_ => false)
    if (res) {
      groupsList.value = res.data.list
    }
  }
  onMounted(getGroups)
  return {
    form,
    item,
    getDetail,
    groupsList,
  }
}

export function useSubmit (form, id) {
  const root = ref(null)
  const router = useRouter()
  const rules = reactive({
    username: [{ required: true, message: T('ParamRequired', { param: T('Username') }) }],
    // email: [{ required: true, message: T('ParamRequired', { param: T('Email') }) }],
    group_id: [{ required: true, message: T('ParamRequired', { param: T('Group') }) }],
    // nickname: [{ required: true, message: '昵称是必须的' }],
    status: [{ required: true, message: T('ParamRequired', { param: T('Status') }) }],
  })

  const validate = async () => {
    const res = await root.value.validate().catch(err => false)
    return res
  }

  // On backend error the axios interceptor (utils/request.js) already
  // shows the server's `res.message` toast and rejects the promise,
  // turning `res` into `false` here. Returning false is enough — adding
  // another `OperationFailed` toast would duplicate the real error.
  const submitCreate = async () => {
    const res = await create(form.value).catch(_ => false)
    return !!res && res.code === 0
  }

  const submitUpdate = async () => {
    const res = await update(form.value).catch(_ => false)
    return !!res && res.code === 0
  }
  const submitFunc = id > 0 ? submitUpdate : submitCreate

  const submit = async () => {
    const v = await validate()
    if (!v) {
      return
    }

    const res = await submitFunc()
    if (res) {
      ElMessage.success(T('OperationSuccess'))
      router.back()
    }
  }

  const cancel = () => {
    router.back()
  }

  return {
    root,
    rules,
    validate,
    submit,
    cancel,
  }
}


