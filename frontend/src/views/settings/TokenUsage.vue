<script setup>
import { ref, inject, onMounted, computed, watch, nextTick } from 'vue'
import { ElMessage } from 'element-plus'
import * as echarts from 'echarts'
import { useThemeStore } from '@/stores/theme'

const api = inject('api')
const themeStore = useThemeStore()

const loading = ref(true)
const error = ref(false)
const records = ref([])
// 日期范围选择器绑定值
const dateRange = ref([
  new Date(Date.now() - 30 * 24 * 60 * 60 * 1000),
  new Date()
])

// 计算属性：开始日期
const startDate = computed(() => dateRange.value?.[0] || new Date(Date.now() - 30 * 24 * 60 * 60 * 1000))
// 计算属性：结束日期
const endDate = computed(() => dateRange.value?.[1] || new Date())

// 聚合数据
const aggregatedData = computed(() => {
  if (!records.value || records.value.length === 0) return null

  const byModel = {}
  const byDate = {}
  const byDateModel = {}
  let totalPrompt = 0
  let totalCompletion = 0
  let totalCalls = 0

  records.value.forEach(r => {
    const pt = r.prompt_tokens
    const ct = r.completion_tokens
    const calls = r.call_count
    const providerId = r.provider_id || 'default'
    totalPrompt += pt
    totalCompletion += ct
    totalCalls += calls

    // 按模型聚合
    const modelKey = `${providerId}:${r.model}`
    if (!byModel[modelKey]) {
      byModel[modelKey] = {
        model: r.model,
        provider_id: providerId,
        prompt_tokens: 0,
        completion_tokens: 0,
        call_count: 0
      }
    }
    byModel[modelKey].prompt_tokens += pt
    byModel[modelKey].completion_tokens += ct
    byModel[modelKey].call_count += calls

    // 按日期聚合
    if (!byDate[r.date]) {
      byDate[r.date] = { prompt_tokens: 0, completion_tokens: 0, call_count: 0 }
    }
    byDate[r.date].prompt_tokens += pt
    byDate[r.date].completion_tokens += ct
    byDate[r.date].call_count += calls

    // 按日期+模型聚合
    if (!byDateModel[r.date]) {
      byDateModel[r.date] = {}
    }
    if (!byDateModel[r.date][modelKey]) {
      byDateModel[r.date][modelKey] = {
        model: r.model,
        provider_id: providerId,
        prompt_tokens: 0,
        completion_tokens: 0,
        call_count: 0
      }
    }
    byDateModel[r.date][modelKey].prompt_tokens += pt
    byDateModel[r.date][modelKey].completion_tokens += ct
    byDateModel[r.date][modelKey].call_count += calls
  })

  return {
    total_prompt_tokens: totalPrompt,
    total_completion_tokens: totalCompletion,
    total_calls: totalCalls,
    by_model: byModel,
    by_date: byDate,
    by_date_model: byDateModel
  }
})

// 按模型表格数据
const byModelTableData = computed(() => {
  if (!aggregatedData.value?.by_model) return []
  return Object.entries(aggregatedData.value.by_model).map(([key, stats]) => ({
    key,
    model: stats.model,
    provider_id: stats.provider_id,
    prompt_tokens: stats.prompt_tokens,
    completion_tokens: stats.completion_tokens,
    call_count: stats.call_count
  }))
})

// 按日期表格数据
const byDateTableData = computed(() => {
  if (!aggregatedData.value?.by_date) return []
  return Object.entries(aggregatedData.value.by_date)
    .map(([date, stats]) => ({
      key: date,
      date,
      prompt_tokens: stats.prompt_tokens,
      completion_tokens: stats.completion_tokens,
      call_count: stats.call_count
    }))
    .sort((a, b) => b.date.localeCompare(a.date))
})

// 分页状态
const byModelPage = ref(1)
const byDatePage = ref(1)

// 分页后的数据
const pagedByModelData = computed(() => {
  const start = (byModelPage.value - 1) * 10
  return byModelTableData.value.slice(start, start + 10)
})
const pagedByDateData = computed(() => {
  const start = (byDatePage.value - 1) * 10
  return byDateTableData.value.slice(start, start + 10)
})

// 格式化数字（紧凑格式）
function formatCompact(num) {
  if (num >= 1_000_000) {
    return `${(num / 1_000_000).toFixed(1)}M`
  } else if (num >= 1_000) {
    return `${(num / 1_000).toFixed(0)}K`
  }
  return num.toString()
}

// 日期格式化
function formatDate(date) {
  const d = new Date(date)
  return d.toISOString().split('T')[0]
}

// 加载数据
async function fetchData() {
  loading.value = true
  error.value = false
  try {
    const params = {
      start_date: formatDate(startDate.value),
      end_date: formatDate(endDate.value)
    }
    const data = await api.getTokenUsageDetails(params)
    records.value = data || []
  } catch (err) {
    console.error('Failed to load token usage:', err)
    ElMessage.error('加载 Token 使用量失败')
    records.value = []
    error.value = true
  } finally {
    loading.value = false
  }
}

// 日期变化处理
function handleDateChange(val) {
  if (!val || val.length !== 2) return
  dateRange.value = val
  fetchData()
}

onMounted(() => {
  fetchData()
})

// 图表实例
const modelTrendChart = ref(null)
const tokenTypeChart = ref(null)
let modelTrendChartInstance = null
let tokenTypeChartInstance = null

// 初始化或获取图表实例
function getChartInstance(domRef) {
  if (!domRef) return null
  // 先看已有实例是否还在
  if (domRef._echartsInstance) {
    return domRef._echartsInstance
  }
  const inst = echarts.init(domRef)
  domRef._echartsInstance = inst
  return inst
}

// 更新图表 — 在数据变化后调用
watch(aggregatedData, () => {
  nextTick(() => {
    updateCharts()
  })
}, { deep: true })

// 主题变化时也需要重绘图表
watch(() => themeStore.actualTheme, () => {
  nextTick(() => {
    // 销毁旧实例，让重新创建时用新主题颜色
    if (modelTrendChart.value && modelTrendChart.value._echartsInstance) {
      modelTrendChart.value._echartsInstance.dispose()
      modelTrendChart.value._echartsInstance = null
    }
    if (tokenTypeChart.value && tokenTypeChart.value._echartsInstance) {
      tokenTypeChart.value._echartsInstance.dispose()
      tokenTypeChart.value._echartsInstance = null
    }
    modelTrendChartInstance = null
    tokenTypeChartInstance = null
    updateCharts()
  })
})

// 根据当前主题获取 ECharts 文字/轴线颜色
function getChartTheme() {
  const isDark = themeStore.actualTheme === 'dark'
  return {
    textColor: isDark ? 'rgba(255,255,255,0.85)' : '#1f1f1f',
    axisLabelColor: isDark ? 'rgba(255,255,255,0.65)' : '#606266',
    splitLineColor: isDark ? 'rgba(255,255,255,0.06)' : 'rgba(0,0,0,0.04)',
    legendColor: isDark ? 'rgba(255,255,255,0.85)' : '#333',
    tooltipBg: isDark ? '#1c2127' : '#fff',
    tooltipColor: isDark ? 'rgba(255,255,255,0.85)' : '#333',
  }
}

function updateCharts() {
  if (!aggregatedData.value) return
  const ct = getChartTheme()

  // ===== 模型趋势图 =====
  if (modelTrendChart.value) {
    modelTrendChartInstance = getChartInstance(modelTrendChart.value)

    // 构建日期列表
    const allDates = []
    const cur = new Date(startDate.value)
    const end = new Date(endDate.value)
    while (cur <= end) {
      allDates.push(formatDate(cur))
      cur.setDate(cur.getDate() + 1)
    }

    // 获取所有模型 key
    const allModels = new Set()
    Object.values(aggregatedData.value.by_date_model).forEach(dayMap => {
      Object.keys(dayMap).forEach(key => allModels.add(key))
    })

    // 构建系列数据
    const series = Array.from(allModels).map(modelKey => {
      const data = allDates.map(date => {
        const dayData = aggregatedData.value.by_date_model[date]?.[modelKey]
        return dayData?.prompt_tokens || 0
      })
      return {
        name: modelKey,
        type: 'line',
        smooth: true,
        symbolSize: 4,
        lineStyle: { width: 2 },
        data
      }
    })

    modelTrendChartInstance.setOption({
      textStyle: { color: ct.textColor },
      tooltip: {
        trigger: 'axis',
        backgroundColor: ct.tooltipBg,
        textStyle: { color: ct.tooltipColor }
      },
      legend: {
        data: Array.from(allModels),
        top: 0,
        textStyle: { color: ct.legendColor }
      },
      grid: { left: '3%', right: '4%', bottom: '8%', top: '50px', containLabel: true },
      xAxis: {
        type: 'category',
        boundaryGap: false,
        data: allDates,
        axisLabel: {
          color: ct.axisLabelColor,
          rotate: 60,
          interval: 'auto',
          formatter: (val) => {
            const d = new Date(val)
            return `${d.getMonth() + 1}-${d.getDate()}`
          }
        },
        axisLine: { lineStyle: { color: ct.splitLineColor } },
        splitLine: { show: false }
      },
      yAxis: {
        type: 'value',
        axisLabel: { color: ct.axisLabelColor, formatter: formatCompact },
        splitLine: { lineStyle: { color: ct.splitLineColor } }
      },
      series
    }, true)
  }

  // ===== Token 类型趋势图 =====
  if (tokenTypeChart.value) {
    tokenTypeChartInstance = getChartInstance(tokenTypeChart.value)

    const allDates = []
    const cur = new Date(startDate.value)
    const end = new Date(endDate.value)
    while (cur <= end) {
      allDates.push(formatDate(cur))
      cur.setDate(cur.getDate() + 1)
    }

    const promptData = allDates.map(date => aggregatedData.value.by_date[date]?.prompt_tokens || 0)
    const completionData = allDates.map(date => aggregatedData.value.by_date[date]?.completion_tokens || 0)
    const totalData = allDates.map((date, i) => promptData[i] + completionData[i])

    tokenTypeChartInstance.setOption({
      textStyle: { color: ct.textColor },
      tooltip: {
        trigger: 'axis',
        backgroundColor: ct.tooltipBg,
        textStyle: { color: ct.tooltipColor }
      },
      legend: {
        data: ['输入 Token', '输出 Token', '总 Token'],
        top: 0,
        textStyle: { color: ct.legendColor }
      },
      grid: { left: '3%', right: '4%', bottom: '8%', top: '50px', containLabel: true },
      xAxis: {
        type: 'category',
        boundaryGap: false,
        data: allDates,
        axisLabel: {
          color: ct.axisLabelColor,
          rotate: 60,
          interval: 'auto',
          formatter: (val) => {
            const d = new Date(val)
            return `${d.getMonth() + 1}-${d.getDate()}`
          }
        },
        axisLine: { lineStyle: { color: ct.splitLineColor } },
        splitLine: { show: false }
      },
      yAxis: {
        type: 'value',
        axisLabel: { color: ct.axisLabelColor, formatter: formatCompact },
        splitLine: { lineStyle: { color: ct.splitLineColor } }
      },
      series: [
        { name: '输入 Token', type: 'line', smooth: true, symbolSize: 4, lineStyle: { width: 2 }, data: promptData, itemStyle: { color: '#1677ff' } },
        { name: '输出 Token', type: 'line', smooth: true, symbolSize: 4, lineStyle: { width: 2 }, data: completionData, itemStyle: { color: '#52c41a' } },
        { name: '总 Token', type: 'line', smooth: true, symbolSize: 4, lineStyle: { width: 2 }, data: totalData, itemStyle: { color: '#fa8c16' } }
      ]
    }, true)
  }
}

// 窗口大小变化时重绘图表
onMounted(() => {
  window.addEventListener('resize', () => {
    modelTrendChartInstance?.resize()
    tokenTypeChartInstance?.resize()
  })
})
</script>

<template>
  <div class="token-usage-page">
    <!-- 工具栏 -->
    <div class="toolbar">
      <el-date-picker
        v-model="dateRange"
        type="daterange"
        range-separator="至"
        start-placeholder="开始日期"
        end-placeholder="结束日期"
        :disabled-date="(time) => time.getTime() > Date.now()"
        @change="handleDateChange"
      />
    </div>

    <!-- 加载状态 -->
    <div v-if="loading" class="loading">
      <el-icon class="is-loading"><Loading /></el-icon>
      <span>加载中...</span>
    </div>

    <!-- 错误状态 -->
    <div v-else-if="error && records.length === 0" class="error">
      <el-icon><WarningFilled /></el-icon>
      <span>加载失败</span>
      <el-button @click="fetchData">重新加载</el-button>
    </div>

    <!-- 数据内容 -->
    <div v-else class="content">
      <!-- 汇总卡片 -->
      <div v-if="aggregatedData" class="summary-cards">
        <el-card class="summary-card">
          <div class="card-value">{{ formatCompact(aggregatedData.total_calls) }}</div>
          <div class="card-label">总调用次数</div>
        </el-card>
        <el-card class="summary-card">
          <div class="card-value">{{ formatCompact(aggregatedData.total_prompt_tokens) }}</div>
          <div class="card-label">输入 Token</div>
        </el-card>
        <el-card class="summary-card">
          <div class="card-value">{{ formatCompact(aggregatedData.total_completion_tokens) }}</div>
          <div class="card-label">输出 Token</div>
        </el-card>
        <el-card class="summary-card">
          <div class="card-value">{{ formatCompact(aggregatedData.total_prompt_tokens + aggregatedData.total_completion_tokens) }}</div>
          <div class="card-label">总 Token</div>
        </el-card>
      </div>

      <!-- 图表 -->
      <div class="charts-row">
        <el-card class="chart-card">
          <template #header>
            <span class="chart-title">模型趋势</span>
          </template>
          <div ref="modelTrendChart" class="chart-container"></div>
        </el-card>
        <el-card class="chart-card">
          <template #header>
            <span class="chart-title">Token 类型趋势</span>
          </template>
          <div ref="tokenTypeChart" class="chart-container"></div>
        </el-card>
      </div>

      <!-- 数据表格 -->
      <div v-if="byModelTableData.length > 0 || byDateTableData.length > 0" class="tables">
        <el-card v-if="byModelTableData.length > 0" class="table-card">
          <template #header>
            <span>按模型统计</span>
          </template>
          <el-table :data="pagedByModelData" size="small" stripe>
            <el-table-column prop="model" label="模型" />
            <el-table-column prop="prompt_tokens" label="输入 Token" :formatter="(row) => formatCompact(row.prompt_tokens)" sortable />
            <el-table-column prop="completion_tokens" label="输出 Token" :formatter="(row) => formatCompact(row.completion_tokens)" sortable />
            <el-table-column label="总 Token" :formatter="(row) => formatCompact(row.prompt_tokens + row.completion_tokens)" sortable />
            <el-table-column prop="call_count" label="调用次数" :formatter="(row) => formatCompact(row.call_count)" sortable />
          </el-table>
          <el-pagination
            v-if="byModelTableData.length > 10"
            :current-page="byModelPage"
            :page-size="10"
            :total="byModelTableData.length"
            layout="prev, pager, next"
            @current-change="(p) => byModelPage = p"
            style="margin-top: 12px; justify-content: center;"
          />
        </el-card>

        <el-card v-if="byDateTableData.length > 0" class="table-card">
          <template #header>
            <span>按日期统计</span>
          </template>
          <el-table :data="pagedByDateData" size="small" stripe>
            <el-table-column prop="date" label="日期" />
            <el-table-column prop="prompt_tokens" label="输入 Token" :formatter="(row) => formatCompact(row.prompt_tokens)" sortable />
            <el-table-column prop="completion_tokens" label="输出 Token" :formatter="(row) => formatCompact(row.completion_tokens)" sortable />
            <el-table-column label="总 Token" :formatter="(row) => formatCompact(row.prompt_tokens + row.completion_tokens)" sortable />
            <el-table-column prop="call_count" label="调用次数" :formatter="(row) => formatCompact(row.call_count)" sortable />
          </el-table>
          <el-pagination
            v-if="byDateTableData.length > 10"
            :current-page="byDatePage"
            :page-size="10"
            :total="byDateTableData.length"
            layout="prev, pager, next"
            @current-change="(p) => byDatePage = p"
            style="margin-top: 12px; justify-content: center;"
          />
        </el-card>
      </div>

      <!-- 空状态 -->
      <div v-else class="empty">
        <el-empty description="暂无数据" />
      </div>
    </div>
  </div>
</template>

<style scoped>
.token-usage-page {
  padding: 16px;
}

.toolbar {
  margin-bottom: 20px;
}

.loading, .error {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 60px;
  color: var(--text-muted);
}

.error {
  flex-direction: column;
}

.content {
  flex: 1;
}

.summary-cards {
  display: flex;
  gap: 16px;
  flex-wrap: wrap;
  margin-bottom: 24px;
}

.summary-card {
  flex: 1 1 140px;
  min-width: 120px;
}

.card-value {
  font-size: 24px;
  font-weight: 600;
  color: var(--text-primary);
  text-align: center;
  margin-bottom: 4px;
}

.card-label {
  font-size: 13px;
  color: var(--text-muted);
  text-align: center;
}

.charts-row {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 16px;
  margin-bottom: 24px;
}

@media (max-width: 768px) {
  .charts-row {
    grid-template-columns: minmax(0, 1fr);
  }
}

.chart-card {
  margin-bottom: 24px;
}

.chart-title {
  font-weight: 500;
  color: var(--text-primary);
}

.chart-container {
  height: 300px;
}

.tables {
  display: flex;
  flex-direction: column;
  gap: 24px;
}

.table-card {
  flex: 1;
}
</style>