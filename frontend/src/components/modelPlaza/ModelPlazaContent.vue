<template>
  <div class="space-y-5">
    <!-- 页头(独立形态下展示标题;后台形态 AppHeader 已有页面标题) -->
    <div v-if="!embedded">
      <h1 class="text-2xl font-bold tracking-tight text-gray-900 dark:text-white sm:text-3xl">{{ t('modelPlaza.title') }}</h1>
      <p class="mt-1.5 text-sm text-gray-500 dark:text-dark-400">{{ t('modelPlaza.description') }}</p>
    </div>

    <!-- 全局价格说明(管理员配置,Markdown) -->
    <div
      v-if="descriptionHtml"
      class="plaza-description rounded-2xl border border-gray-100 bg-white px-5 py-4 text-sm shadow-card dark:border-dark-700/50 dark:bg-dark-800/50"
      v-html="descriptionHtml"
    ></div>

    <!-- 未登录提示 -->
    <p
      v-if="!isAuthenticated"
      class="flex items-center gap-1.5 text-xs text-gray-400 dark:text-dark-500"
    >
      <Icon name="infoCircle" size="xs" class="h-3.5 w-3.5" />
      {{ t('modelPlaza.anonymousHint') }}
    </p>

    <!-- 加载/错误/空 -->
    <div v-if="loading" class="flex min-h-[240px] items-center justify-center">
      <div class="h-8 w-8 animate-spin rounded-full border-2 border-primary-600/25 border-t-primary-600 dark:border-primary-400/25 dark:border-t-primary-400"></div>
    </div>
    <div
      v-else-if="error"
      class="rounded-2xl border border-red-200 bg-red-50 px-5 py-8 text-center text-sm text-red-600 dark:border-red-500/30 dark:bg-red-500/10 dark:text-red-300"
    >
      {{ t('modelPlaza.loadFailed') }}
    </div>
    <template v-else>
      <!-- 顶部统计概览(New-API 模型广场风格) -->
      <div
        v-if="(props.response?.groups?.length ?? 0) > 0"
        class="grid grid-cols-2 gap-3 sm:grid-cols-4"
      >
        <div class="rounded-xl border border-gray-100 bg-white px-4 py-3 shadow-card dark:border-dark-700/60 dark:bg-dark-800/50">
          <div class="text-xs font-medium text-gray-500 dark:text-dark-400">{{ t('modelPlaza.stats.totalModels') }}</div>
          <div class="mt-1 flex items-baseline gap-1.5 font-mono text-xl font-bold text-gray-900 dark:text-white">
            <span>{{ summaryStats.filteredModels }}</span>
            <span
              v-if="summaryStats.filteredModels !== summaryStats.totalModels"
              class="text-xs font-normal text-gray-400 dark:text-dark-500"
            >/ {{ summaryStats.totalModels }}</span>
          </div>
        </div>
        <div class="rounded-xl border border-gray-100 bg-white px-4 py-3 shadow-card dark:border-dark-700/60 dark:bg-dark-800/50">
          <div class="text-xs font-medium text-gray-500 dark:text-dark-400">{{ t('modelPlaza.stats.totalGroups') }}</div>
          <div class="mt-1 font-mono text-xl font-bold text-gray-900 dark:text-white">
            {{ summaryStats.totalGroups }}
          </div>
        </div>
        <div class="rounded-xl border border-gray-100 bg-white px-4 py-3 shadow-card dark:border-dark-700/60 dark:bg-dark-800/50">
          <div class="text-xs font-medium text-gray-500 dark:text-dark-400">{{ t('modelPlaza.stats.totalPlatforms') }}</div>
          <div class="mt-1 font-mono text-xl font-bold text-gray-900 dark:text-white">
            {{ summaryStats.totalPlatforms }}
          </div>
        </div>
        <div class="rounded-xl border border-gray-100 bg-white px-4 py-3 shadow-card dark:border-dark-700/60 dark:bg-dark-800/50">
          <div class="text-xs font-medium text-gray-500 dark:text-dark-400">{{ t('modelPlaza.stats.lowestRate') }}</div>
          <div class="mt-1 font-mono text-xl font-bold text-primary-600 dark:text-primary-400">
            {{ summaryStats.lowestRate != null ? `${summaryStats.lowestRate}x` : '-' }}
          </div>
        </div>
      </div>

      <!-- 筛选区:平台 → 分组 → 倍率 -->
      <PlazaFilterBar
        :platforms="platforms"
        :groups="groupOptions"
        :rates="rates"
        :platform="selectedPlatform"
        :group-id="selectedGroupId"
        :rate="selectedRate"
        :billing-mode="selectedBillingMode"
        :view-mode="viewMode"
        :search="searchQuery"
        @update:platform="selectedPlatform = $event"
        @update:group-id="selectedGroupId = $event"
        @update:rate="selectedRate = $event"
        @update:billing-mode="selectedBillingMode = $event"
        @update:view-mode="viewMode = $event"
        @update:search="searchQuery = $event"
      />

      <!-- 分组分节表格视图 或 New-API 风格模型卡片网格视图 -->
      <template v-if="filteredGroups.length > 0">
        <div v-if="viewMode === 'table'" class="space-y-5">
          <PlazaGroupSection v-for="g in filteredGroups" :key="g.id" :group="g" />
        </div>

        <!-- New-API 风格模型卡片视图 -->
        <div v-else class="grid grid-cols-1 gap-3.5 sm:grid-cols-2 lg:grid-cols-3">
          <div
            v-for="card in modelCards"
            :key="card.key"
            class="group flex flex-col justify-between rounded-xl border border-gray-200/80 bg-white p-4 shadow-card transition-all hover:border-primary-400/60 hover:shadow-md dark:border-dark-700/70 dark:bg-dark-800/60 dark:hover:border-primary-500/50"
          >
            <div>
              <!-- 顶部:平台图标 + 模型名(点击复制) + 最低倍率徽章 -->
              <div class="flex items-start justify-between gap-2">
                <button
                  type="button"
                  class="flex min-w-0 items-center gap-2 text-left"
                  :title="t('modelPlaza.cards.copyModel')"
                  @click="copyModelName(card.name, card.key)"
                >
                  <PlatformIcon :platform="card.platform as GroupPlatform" size="sm" class="shrink-0" />
                  <span class="truncate font-mono text-sm font-semibold text-gray-900 transition-colors group-hover:text-primary-600 dark:text-white dark:group-hover:text-primary-400">
                    {{ card.name }}
                  </span>
                  <Icon
                    :name="copiedCardKey === card.key ? 'check' : 'copy'"
                    size="xs"
                    class="h-3.5 w-3.5 shrink-0 transition-colors"
                    :class="
                      copiedCardKey === card.key
                        ? 'text-emerald-500'
                        : 'text-gray-400 group-hover:text-primary-500 dark:text-dark-400'
                    "
                  />
                </button>
                <span
                  class="shrink-0 rounded-md bg-primary-50 px-2 py-0.5 font-mono text-xs font-bold text-primary-700 dark:bg-primary-900/30 dark:text-primary-300"
                >
                  {{ card.lowestRate }}x
                </span>
              </div>

              <!-- 平台与计费模式标签 -->
              <div class="mt-2 flex flex-wrap items-center gap-1.5">
                <span
                  class="inline-flex items-center rounded-md px-1.5 py-0.5 text-[11px] font-medium"
                  :class="platformBadgeLightClass(card.platform)"
                >
                  {{ platformLabel(card.platform) }}
                </span>
                <span
                  class="rounded-md bg-gray-100 px-1.5 py-0.5 text-[11px] font-medium text-gray-600 dark:bg-dark-700/70 dark:text-dark-300"
                >
                  {{ card.billingLabel }}
                </span>
              </div>

              <!-- 价格详情区 -->
              <div class="mt-3 rounded-lg bg-gray-50/90 px-3 py-2.5 text-xs dark:bg-dark-900/50">
                <template v-if="card.isTokenMode">
                  <div class="grid grid-cols-2 gap-2">
                    <div>
                      <div class="text-[11px] text-gray-400 dark:text-dark-500">
                        {{ t('modelPlaza.cards.paidInput') }}
                      </div>
                      <div class="mt-0.5 font-mono text-sm font-semibold text-gray-900 dark:text-white">
                        {{ card.paidInput }}
                        <span class="text-[10px] font-normal text-gray-400">/ 1M</span>
                      </div>
                    </div>
                    <div>
                      <div class="text-[11px] text-gray-400 dark:text-dark-500">
                        {{ t('modelPlaza.cards.paidOutput') }}
                      </div>
                      <div class="mt-0.5 font-mono text-sm font-semibold text-gray-900 dark:text-white">
                        {{ card.paidOutput }}
                        <span class="text-[10px] font-normal text-gray-400">/ 1M</span>
                      </div>
                    </div>
                  </div>
                  <div
                    v-if="card.officialInput !== '-' || card.officialOutput !== '-'"
                    class="mt-1.5 border-t border-gray-200/70 pt-1.5 font-mono text-[11px] text-gray-400 dark:border-dark-700/60 dark:text-dark-500"
                  >
                    {{ t('modelPlaza.cards.officialRef', { input: card.officialInput, output: card.officialOutput }) }}
                  </div>
                </template>
                <template v-else>
                  <div class="flex items-center justify-between">
                    <span class="text-[11px] text-gray-400 dark:text-dark-500">
                      {{ t('modelPlaza.cards.perRequestPrice') }}
                    </span>
                    <span class="font-mono text-sm font-semibold text-gray-900 dark:text-white">
                      {{ card.perRequestDisplay }}
                    </span>
                  </div>
                </template>
              </div>
            </div>

            <!-- 底部:所属分组标签(点击可按该分组筛选) -->
            <div class="mt-3 flex flex-wrap items-center gap-1.5 border-t border-gray-100 pt-2.5 dark:border-dark-700/50">
              <span class="text-[11px] text-gray-400 dark:text-dark-500">
                {{ t('modelPlaza.cards.availableInGroups') }}:
              </span>
              <button
                v-for="grp in card.groups"
                :key="grp.id"
                type="button"
                class="inline-flex items-center gap-1 rounded-md bg-gray-100 px-2 py-0.5 text-[11px] font-medium text-gray-700 transition-colors hover:bg-primary-50 hover:text-primary-700 dark:bg-dark-700/70 dark:text-dark-200 dark:hover:bg-primary-900/30 dark:hover:text-primary-300"
                @click="selectedGroupId = selectedGroupId === grp.id ? 'all' : grp.id"
              >
                <span>{{ grp.name }}</span>
                <span class="font-mono text-[10px] text-primary-600 dark:text-primary-400">{{ grp.rate }}x</span>
              </button>
            </div>
          </div>
        </div>
      </template>
      <div
        v-else
        class="rounded-2xl border border-dashed border-gray-300 px-5 py-12 text-center text-sm text-gray-500 dark:border-dark-600 dark:text-dark-400"
      >
        <div>{{ searchActive ? t('modelPlaza.noSearchResult') : t('modelPlaza.empty') }}</div>
        <p
          v-if="!searchActive && authStore.isAdmin"
          class="mt-2 text-xs text-gray-400 dark:text-dark-500"
        >
          {{ t('modelPlaza.emptyAdminHint') }}
        </p>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { marked } from 'marked'
import DOMPurify from 'dompurify'
import Icon from '@/components/icons/Icon.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import PlazaFilterBar, { type PlazaBillingFilter, type PlazaViewMode } from './PlazaFilterBar.vue'
import PlazaGroupSection from './PlazaGroupSection.vue'
import type { ModelPlazaGroup, ModelPlazaResponse, PlazaModel } from '@/api/modelPlaza'
import type { GroupPlatform } from '@/types'
import { BILLING_MODE_TOKEN, BILLING_MODE_IMAGE, BILLING_MODE_VIDEO } from '@/constants/channel'
import { formatScaled } from '@/utils/pricing'
import { platformBadgeLightClass, platformLabel } from '@/utils/platformColors'
import { useAuthStore } from '@/stores/auth'
import { useAppStore } from '@/stores/app'

const props = defineProps<{
  response: ModelPlazaResponse | null
  loading: boolean
  error?: boolean
  /** 后台内嵌形态(AppLayout 内):隐藏页头。 */
  embedded?: boolean
}>()

const { t } = useI18n()
const authStore = useAuthStore()
const isAuthenticated = computed(() => authStore.isAuthenticated)

const selectedPlatform = ref<string>('all')
const selectedGroupId = ref<number | 'all'>('all')
const selectedRate = ref<number | 'all'>('all')
const selectedBillingMode = ref<PlazaBillingFilter>('all')
const viewMode = ref<PlazaViewMode>('table')
const searchQuery = ref('')
const copiedCardKey = ref<string | null>(null)

const searchActive = computed(
  () => searchQuery.value.trim() !== '' || selectedBillingMode.value !== 'all'
)

const descriptionHtml = computed(() => {
  const md = props.response?.description?.trim()
  if (!md) return ''
  return DOMPurify.sanitize(marked.parse(md) as string)
})

/** 生效倍率 = 用户专属倍率 ?? 分组默认倍率。 */
function effectiveRate(g: ModelPlazaGroup): number {
  return g.user_rate_multiplier ?? g.rate_multiplier
}

function isTokenModel(m: PlazaModel): boolean {
  return (m.pricing?.billing_mode || BILLING_MODE_TOKEN) === BILLING_MODE_TOKEN
}

function matchesBillingFilter(m: PlazaModel, filter: PlazaBillingFilter): boolean {
  if (filter === 'all') return true
  if (filter === 'token') return isTokenModel(m)
  return !isTokenModel(m)
}

const platforms = computed(() =>
  [...new Set((props.response?.groups ?? []).map((g) => g.platform).filter(Boolean))].sort()
)

const groupOptions = computed(() =>
  (props.response?.groups ?? []).map((g) => ({
    id: g.id,
    name: g.name,
    platform: g.platform,
    rate: effectiveRate(g)
  }))
)

/** 全量生效倍率;当前组合下不可用的项由 FilterBar 置灰而非隐藏。 */
const rates = computed(() =>
  [...new Set((props.response?.groups ?? []).map(effectiveRate))].sort((a, b) => a - b)
)

/** 数据刷新后选中的倍率可能不复存在,重置为全部。 */
watch(rates, (list) => {
  if (selectedRate.value !== 'all' && !list.includes(selectedRate.value)) {
    selectedRate.value = 'all'
  }
})

const filteredGroups = computed(() => {
  let groups = props.response?.groups ?? []
  if (selectedPlatform.value !== 'all') {
    groups = groups.filter((g) => g.platform === selectedPlatform.value)
  }
  if (selectedGroupId.value !== 'all') {
    groups = groups.filter((g) => g.id === selectedGroupId.value)
  }
  if (selectedRate.value !== 'all') {
    groups = groups.filter((g) => effectiveRate(g) === selectedRate.value)
  }
  // 模型名搜索:分组内只留命中的模型,整组无命中则隐藏该分组。
  const q = searchQuery.value.trim().toLowerCase()
  if (q || selectedBillingMode.value !== 'all') {
    groups = groups
      .map((g) => ({
        ...g,
        models: g.models.filter(
          (m) =>
            (!q || m.name.toLowerCase().includes(q)) &&
            matchesBillingFilter(m, selectedBillingMode.value)
        )
      }))
      .filter((g) => g.models.length > 0)
  }
  // 专属倍率会改变生效值,不能只依赖后端按默认倍率的排序。
  return [...groups].sort(
    (a, b) => effectiveRate(a) - effectiveRate(b) || a.name.localeCompare(b.name)
  )
})

const summaryStats = computed(() => {
  const allGroups = props.response?.groups ?? []
  const allModelKeys = new Set<string>()
  for (const g of allGroups) {
    for (const m of g.models) {
      allModelKeys.add(`${m.platform}:${m.name}`)
    }
  }
  const filteredModelKeys = new Set<string>()
  for (const g of filteredGroups.value) {
    for (const m of g.models) {
      filteredModelKeys.add(`${m.platform}:${m.name}`)
    }
  }
  return {
    totalModels: allModelKeys.size,
    filteredModels: filteredModelKeys.size,
    totalGroups: allGroups.length,
    totalPlatforms: platforms.value.length,
    lowestRate: rates.value.length > 0 ? rates.value[0] : null
  }
})

interface PlazaModelCard {
  key: string
  name: string
  platform: string
  isTokenMode: boolean
  billingLabel: string
  lowestRate: number
  paidInput: string
  paidOutput: string
  officialInput: string
  officialOutput: string
  perRequestDisplay: string
  groups: Array<{ id: number; name: string; rate: number }>
}

const PER_MILLION = 1_000_000
const MIN_DECIMALS = 2

function resolveModelEffectiveRate(g: ModelPlazaGroup, m: PlazaModel): number {
  const mode = m.pricing?.billing_mode || BILLING_MODE_TOKEN
  if (mode === BILLING_MODE_VIDEO && g.video_rate_independent) {
    return Math.max(0, g.video_rate_multiplier ?? 1)
  }
  if (mode === BILLING_MODE_IMAGE && g.image_rate_independent) {
    return g.image_rate_multiplier ?? 1
  }
  return effectiveRate(g)
}

const modelCards = computed<PlazaModelCard[]>(() => {
  const map = new Map<string, { model: PlazaModel; groupRef: ModelPlazaGroup; rate: number; groups: Array<{ id: number; name: string; rate: number }> }>()
  for (const g of filteredGroups.value) {
    for (const m of g.models) {
      const key = `${m.platform}:${m.name}`
      const rate = resolveModelEffectiveRate(g, m)
      const existing = map.get(key)
      if (!existing) {
        map.set(key, {
          model: m,
          groupRef: g,
          rate,
          groups: [{ id: g.id, name: g.name, rate }]
        })
      } else {
        if (!existing.groups.some((item) => item.id === g.id)) {
          existing.groups.push({ id: g.id, name: g.name, rate })
        }
        if (rate < existing.rate) {
          existing.rate = rate
          existing.model = m
          existing.groupRef = g
        }
      }
    }
  }

  const cards: PlazaModelCard[] = []
  for (const [key, entry] of map.entries()) {
    const { model: m, rate, groups: grpList } = entry
    const mode = m.pricing?.billing_mode || BILLING_MODE_TOKEN
    const isToken = mode === BILLING_MODE_TOKEN
    const billingLabel = isToken
      ? t('availableChannels.pricing.billingModeToken')
      : mode === BILLING_MODE_IMAGE
        ? t('modelPlaza.table.perImage')
        : t('modelPlaza.table.perRequest')

    const paidInput =
      m.pricing?.input_price != null
        ? formatScaled(m.pricing.input_price * rate, PER_MILLION, MIN_DECIMALS)
        : '-'
    const paidOutput =
      m.pricing?.output_price != null
        ? formatScaled(m.pricing.output_price * rate, PER_MILLION, MIN_DECIMALS)
        : '-'
    const officialInput =
      m.official_pricing?.input_price != null
        ? formatScaled(m.official_pricing.input_price, PER_MILLION, MIN_DECIMALS)
        : '-'
    const officialOutput =
      m.official_pricing?.output_price != null
        ? formatScaled(m.official_pricing.output_price, PER_MILLION, MIN_DECIMALS)
        : '-'

    const unitSuffix =
      mode === BILLING_MODE_IMAGE
        ? t('modelPlaza.table.perUnitImage')
        : t('modelPlaza.table.perUnitRequest')
    const firstIntervalPrice = m.pricing?.intervals?.find((iv) => iv.per_request_price != null)?.per_request_price
    const baseReqPrice = m.pricing?.per_request_price ?? firstIntervalPrice ?? null
    const perRequestDisplay =
      baseReqPrice != null
        ? `${formatScaled(baseReqPrice * rate, 1, MIN_DECIMALS)} ${unitSuffix}`
        : '-'

    cards.push({
      key,
      name: m.name,
      platform: m.platform,
      isTokenMode: isToken,
      billingLabel,
      lowestRate: rate,
      paidInput,
      paidOutput,
      officialInput,
      officialOutput,
      perRequestDisplay,
      groups: grpList
    })
  }

  return cards.sort((a, b) => {
    if (a.isTokenMode !== b.isTokenMode) return a.isTokenMode ? -1 : 1
    if (a.lowestRate !== b.lowestRate) return a.lowestRate - b.lowestRate
    return a.name.localeCompare(b.name)
  })
})

async function copyModelName(name: string, cardKey: string) {
  if (!name) return
  let ok = false
  if (typeof navigator !== 'undefined' && navigator.clipboard && window.isSecureContext) {
    try {
      await navigator.clipboard.writeText(name)
      ok = true
    } catch {
      ok = false
    }
  }
  if (!ok && typeof document !== 'undefined') {
    const textarea = document.createElement('textarea')
    textarea.value = name
    textarea.setAttribute('readonly', 'true')
    textarea.style.cssText = 'position:fixed;left:0;top:0;width:1px;height:1px;opacity:0;pointer-events:none'
    document.body.appendChild(textarea)
    textarea.select()
    try {
      ok = document.execCommand('copy')
    } catch {
      ok = false
    } finally {
      document.body.removeChild(textarea)
    }
  }
  if (ok) {
    copiedCardKey.value = cardKey
    setTimeout(() => {
      if (copiedCardKey.value === cardKey) copiedCardKey.value = null
    }, 1800)
    try {
      useAppStore().showSuccess(t('modelPlaza.cards.copiedModel', { model: name }))
    } catch {
      // 单测环境无 Pinia 时静默忽略
    }
  }
}
</script>

<style scoped>
.plaza-description {
  line-height: 1.7;
  overflow-wrap: anywhere;
}

.plaza-description :deep(h1),
.plaza-description :deep(h2),
.plaza-description :deep(h3) {
  @apply mb-2 mt-3 font-semibold text-gray-900 first:mt-0 dark:text-white;
}

.plaza-description :deep(p) {
  @apply mb-2 text-gray-700 last:mb-0 dark:text-dark-200;
}

.plaza-description :deep(a) {
  @apply text-primary-600 underline underline-offset-4 hover:text-primary-700 dark:text-primary-300;
}

.plaza-description :deep(ul) {
  @apply mb-2 list-disc pl-5;
}

.plaza-description :deep(ol) {
  @apply mb-2 list-decimal pl-5;
}

.plaza-description :deep(li) {
  @apply mb-0.5 text-gray-700 dark:text-dark-200;
}

.plaza-description :deep(code) {
  @apply rounded bg-gray-100 px-1.5 py-0.5 font-mono text-xs dark:bg-dark-800;
}

.plaza-description :deep(blockquote) {
  @apply my-2 border-l-4 border-gray-300 pl-3 text-gray-600 dark:border-dark-600 dark:text-dark-300;
}
</style>
