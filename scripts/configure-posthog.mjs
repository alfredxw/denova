import assert from 'node:assert/strict'
import fs from 'node:fs/promises'
import { loadEnvFile } from 'node:process'

// This management script never runs in the App and never sends capture events.
// Credentials stay in the ignored repository-root .env or the process environment.
const root = new URL('../', import.meta.url)
try { loadEnvFile(new URL('.env', root)) } catch (error) { if (error.code !== 'ENOENT') throw error }
const key = process.env.POSTHOG_PERSONAL_API_KEY
const origin = 'https://us.posthog.com'
const projectId = 685522
const base = `/api/projects/${projectId}/`
const tag = 'denova-app-schema-2'
const dashboardName = 'Denova App｜产品分析'
const apply = process.argv.includes('--apply')
if (process.argv.slice(2).some(arg => !['--apply', '--verify', '--plan'].includes(arg))) throw new Error('Usage: node scripts/configure-posthog.mjs [--apply | --verify | --plan]')

async function api(path, body, method = body === undefined ? 'GET' : 'POST') {
  if (!path.startsWith(base)) throw new Error('Request is outside the authorized project')
  const response = await fetch(origin + path, {
    method, headers: { Authorization: `Bearer ${key}`, 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
    signal: AbortSignal.timeout(60000), redirect: 'error',
  })
  const raw = await response.text()
  let data
  try { data = JSON.parse(raw) } catch { throw new Error(`PostHog returned non-JSON status ${response.status}`) }
  if (!response.ok) throw new Error(JSON.stringify({ status: response.status, detail: data }).replaceAll(key, '[redacted]'))
  if (data.error || data.query_status?.error) throw new Error(JSON.stringify(data.error || data.query_status.error))
  return data
}

async function query(source) {
  let result = await api(base + 'query/', { query: source, refresh: 'blocking' })
  for (let attempt = 0; result.query_status?.complete === false; attempt++) {
    if (attempt === 15 || !result.query_status.id) throw new Error('Query did not complete within the verification window')
    await new Promise(resolve => setTimeout(resolve, 2000))
    result = await api(base + `query/${encodeURIComponent(result.query_status.id)}/`)
  }
  return result
}

const property = (key, value) => ({ key, type: 'event', value: [value], operator: 'exact' })
const production = [property('source', 'app'), property('schema_version', 2)]
const releaseProperties = [...production, { ...property('app_version', '^v?[0-9]+[.][0-9]+[.][0-9]+$'), operator: 'regex' }]
const user = property('trigger', 'user')
const dates = date_from => ({ date_from, explicitDate: false, excludeIncompletePeriods: false })
const event = (name, math = 'total', properties = [], custom_name = name) => ({ kind: 'EventsNode', event: name, math, properties, custom_name })
const trend = (series, extra = {}) => ({
  kind: 'TrendsQuery', series, version: 4, interval: 'day', dateRange: dates('-30d'),
  properties: production, filterTestAccounts: false,
  trendsFilter: { display: 'ActionsLineGraph', showLegend: true, legendPosition: 'bottom', yAxisStartAtZero: true, metricSummary: 'latest' }, ...extra,
})
const breakdown = key => ({ breakdowns: [{ type: 'event', property: key }], breakdown_type: 'event', breakdown_limit: 10 })
const prodSql = "properties.source = 'app' AND properties.schema_version = 2"
// Version format separates known fork/development suffixes; it is not proof of build provenance.
const releaseVersionSql = "match(toString(properties.app_version), '^v?[0-9]+[.][0-9]+[.][0-9]+$')"
const releaseSql = `${prodSql} AND ${releaseVersionSql}`
const userSql = "properties.trigger = 'user'"
const runSql = "event IN ('feature_completed', 'feature_failed', 'feature_ended')"
const sql = (query, range = '-30d') => ({ kind: 'HogQLQuery', query, filters: { dateRange: dates(range) } })
const insight = (slug, name, description, source) => ({
  slug, name, description,
  query: { kind: source.kind === 'HogQLQuery' ? 'DataTableNode' : 'InsightVizNode', source },
})
const retention = feature => ({
  kind: 'RetentionQuery', version: 3, dateRange: dates('-90d'),
  properties: [...production, property('cohort_origin', 'new_instance')], filterTestAccounts: false,
  retentionFilter: {
    targetEntity: { id: 'app_activated', type: 'events', properties: [property('feature_name', feature)] },
    returningEntity: { id: 'core_active_day', type: 'events', properties: [property('feature_name', feature)] },
    retentionType: 'retention_first_ever_occurrence', retentionReference: 'total',
    period: 'Day', totalIntervals: 31, cumulative: false, timeWindowMode: 'strict_calendar_dates',
    meanRetentionCalculation: 'weighted', dashboardDisplay: 'table_only',
  },
})

// Only completed UTC calendar days enter each retention denominator. Returning
// events are observed through now, even when the selected activation cohort ends earlier.
const retentionSummary = (cohorts, returns) => `
WITH cohorts AS (${cohorts}), returns AS (${returns}), activity AS (
  SELECT cohorts.feature AS feature, cohorts.cohort_day AS cohort_day,
    has(returns.days, cohorts.cohort_day + INTERVAL 1 DAY) AS d1,
    has(returns.days, cohorts.cohort_day + INTERVAL 7 DAY) AS d7,
    has(returns.days, cohorts.cohort_day + INTERVAL 30 DAY) AS d30
  FROM cohorts LEFT JOIN returns ON cohorts.instance = returns.instance AND cohorts.feature = returns.feature
)
SELECT feature,
  ${[1, 7, 30].map(day => `countIf(cohort_day + INTERVAL ${day} DAY < toDate(now())) AS d${day}_eligible,
  countIf(d${day} AND cohort_day + INTERVAL ${day} DAY < toDate(now())) AS d${day}_returned,
  round(100.0 * d${day}_returned / nullIf(d${day}_eligible, 0), 2) AS d${day}_pct`).join(',\n  ')}
FROM activity GROUP BY feature ORDER BY feature`

const insights = [
  insight('active', '01 · DAU / WAU / MAU', '有效用户活动的去重实例数；DAU 按 UTC 日，WAU/MAU 为滚动 7/30 天。排除自动任务和错误事件；人数不等于实名用户数。',
    trend([event('feature_used', 'dau', [user], 'DAU'), event('feature_used', 'weekly_active', [user], 'WAU · 7天'), event('feature_used', 'monthly_active', [user], 'MAU · 30天')])),
  insight('feature-totals', '02 · 各功能使用人数与次数', '所选时间内按功能去重，人数不累加每日 UV。次数是有效阅读、编辑、执行及成功操作的观测次数，不是点击数；自动任务排除。', sql(`
SELECT properties.feature_name AS feature,
       uniqExact(distinct_id) AS instances, count() AS uses,
       countIf(properties.operation = 'read') AS reads,
       countIf(properties.operation = 'edit') AS edits,
       countIf(properties.operation = 'run') AS runs,
       countIf(properties.operation NOT IN ('read', 'edit', 'run')) AS actions
FROM events WHERE {filters} AND ${prodSql} AND ${userSql} AND event = 'feature_used'
GROUP BY feature ORDER BY instances DESC, uses DESC`)),
  insight('feature-daily-users', '03 · 各功能每日使用人数', '每日按匿名实例去重，可按功能观察采用趋势。', trend([event('feature_used', 'dau', [user], '使用人数')], { breakdownFilter: breakdown('feature_name') })),
  insight('feature-daily-uses', '04 · 各功能每日使用次数', '有效使用事件次数；一次较长阅读可产生多个 30 秒里程碑，一次执行只记一次开始。', trend([event('feature_used', 'total', [user], '使用次数')], {
    breakdownFilter: breakdown('feature_name'), trendsFilter: { display: 'ActionsStackedBar', showLegend: true },
  })),
  insight('activation', '05 · 新实例首次激活漏斗', '首次被 PostHog 观察的新实例 → 7 天内首次成功核心工作（写作或游戏）。排除从旧分析迁移的实例。激活是技术完成代理指标，不等于内容质量。', {
    kind: 'FunnelsQuery', version: 2, dateRange: dates('-30d'), properties: [...production, property('cohort_origin', 'new_instance')], filterTestAccounts: false,
    series: [event('app_first_seen'), event('app_activated', 'total', [{ key: 'feature_name', type: 'event', operator: 'exact', value: ['writing', 'game'] }])],
    funnelsFilter: { funnelOrderType: 'ordered', funnelVizType: 'steps', funnelWindowInterval: 7, funnelWindowIntervalUnit: 'day', funnelStepReference: 'total', layout: 'vertical' },
  }),
  insight('writing-retention', '06 · 写作激活留存 · D1–D30', '新实例首次写作激活 → 同一实例再次完成写作核心工作。UTC 严格日历日、非累计；未成熟日期不可当成 0% 留存。默认看近 90 天。', retention('writing')),
  insight('game-retention', '07 · 游戏激活留存 · D1–D30', '新实例首次游戏激活 → 同一实例再次完成游戏核心工作。UTC 严格日历日、非累计；与写作分别计算。默认看近 90 天。', retention('game')),
  insight('retention-summary', '07b · D1 / D7 / D30 成熟队列留存', '按所选时间内首次激活的新实例建立队列，观察截至当前的同功能核心工作。只纳入已经完整度过对应 UTC 日的实例；未成熟分母为 0 时留存率为空。eligible=成熟人数，returned=严格第 N 天回访人数。', sql(retentionSummary(`
SELECT distinct_id AS instance, properties.feature_name AS feature, min(toDate(timestamp)) AS cohort_day
FROM events WHERE {filters} AND ${prodSql} AND event = 'app_activated'
  AND properties.cohort_origin = 'new_instance' AND properties.feature_name IN ('writing', 'game')
GROUP BY instance, feature`, `
SELECT distinct_id AS instance, properties.feature_name AS feature, groupUniqArray(toDate(timestamp)) AS days
FROM events WHERE ${prodSql} AND event = 'core_active_day' AND timestamp <= now()
  AND properties.cohort_origin = 'new_instance' AND properties.feature_name IN ('writing', 'game')
GROUP BY instance, feature`), '-90d')),
  insight('success', '08 · 已结束执行成功率', '正式版本号样本（x.y.z）；fork / 开发版本单列在接入状态。成功 / (成功 + 失败)，按版本、系统、功能与运行时统计。取消、暂停、抢占不进分母；空分母不显示 0%。', sql(`
SELECT properties.app_version AS version, properties.os AS os,
       properties.feature_name AS feature, properties.runtime AS runtime,
       countIf(event = 'feature_completed') AS succeeded,
       countIf(event = 'feature_failed') AS failed,
       uniqExactIf(distinct_id, event = 'feature_failed') AS affected_instances,
       round(100.0 * countIf(event = 'feature_completed') / nullIf(countIf(event IN ('feature_completed', 'feature_failed')), 0), 2) AS success_pct,
       countIf(event = 'feature_ended' AND properties.outcome = 'cancelled') AS cancelled,
       countIf(event = 'feature_ended' AND properties.outcome = 'suspended') AS suspended,
       countIf(event = 'feature_ended' AND properties.outcome = 'preempted') AS preempted
FROM events WHERE {filters} AND ${releaseSql} AND ${userSql} AND ${runSql}
GROUP BY version, os, feature, runtime ORDER BY succeeded + failed DESC`)),
  insight('outcomes', '09 · 执行结果趋势', '按结束时间统计用户执行的成功、失败、取消、暂停、抢占；不把暂停/取消当作失败。', trend([
    event('feature_completed', 'total', [user], '成功'), event('feature_failed', 'total', [user], '失败'),
    ...['cancelled', 'suspended', 'preempted'].map((value, i) => event('feature_ended', 'total', [user, property('outcome', value)], ['取消', '暂停', '抢占'][i])),
  ], { trendsFilter: { display: 'ActionsStackedBar', showLegend: true } })),
  insight('run-cohort', '10 · 开始执行队列的最终状态', '按所选时间内的开始事件建立队列，再用 run_id 查截至当前已收到的终态。未匹配代表仍在运行或事件缺失，不能视为失败；跨日结束不会被漏算。', sql(`
WITH starts AS (
  SELECT properties.run_id AS run_id, any(properties.feature_name) AS feature, any(properties.runtime) AS runtime
  FROM events WHERE {filters} AND ${prodSql} AND ${userSql} AND event = 'feature_used' AND properties.operation = 'run'
  GROUP BY run_id
), endings AS (
  SELECT properties.run_id AS run_id, argMax(properties.outcome, timestamp) AS outcome, count() AS ends
  FROM events WHERE ${prodSql} AND ${userSql} AND ${runSql} AND timestamp <= now()
  GROUP BY run_id
)
SELECT starts.feature AS feature, starts.runtime AS runtime, count() AS admitted,
       countIf(endings.outcome = 'success') AS succeeded, countIf(endings.outcome = 'failure') AS failed,
       countIf(endings.outcome = 'cancelled') AS cancelled, countIf(endings.outcome = 'suspended') AS suspended,
       countIf(endings.outcome = 'preempted') AS preempted,
       countIf(coalesce(endings.ends, 0) = 0) AS unfinished_or_missing
FROM starts LEFT JOIN endings ON starts.run_id = endings.run_id
GROUP BY feature, runtime ORDER BY admitted DESC`)),
  insight('latency', '11 · 完整执行耗时 · P50 / P95', '按功能、运行时与成功/失败分别显示耗时（秒），包含整个执行过程，不是首 Token 延迟。没有样本时无行，不显示 0 秒。', sql(`
SELECT properties.feature_name AS feature, properties.runtime AS runtime, properties.outcome AS outcome,
       count() AS samples, round(quantile(0.5)(toFloat(properties.duration_ms)) / 1000, 2) AS p50_seconds,
       round(quantile(0.95)(toFloat(properties.duration_ms)) / 1000, 2) AS p95_seconds
FROM events WHERE {filters} AND ${prodSql} AND ${userSql}
  AND event IN ('feature_completed', 'feature_failed') AND properties.duration_ms IS NOT NULL
GROUP BY feature, runtime, outcome ORDER BY samples DESC`)),
  insight('latency-trend', '12 · 成功执行每日耗时 · 毫秒', '成功执行 duration_ms 的 P50/P95；低样本百分位波动较大，可结合耗时明细样本数判断。', trend([
    { ...event('feature_completed', 'median', [user], 'P50 · ms'), math_property: 'duration_ms' },
    { ...event('feature_completed', 'p95', [user], 'P95 · ms'), math_property: 'duration_ms' },
  ])),
  insight('error-trend', '13 · 报错次数与受影响人数', '正式版本号样本的 app_error 报告次数及去重受影响实例数。一次故障可能跨层报告；此处不与执行失败次数相加。4xx 不一定是程序缺陷。', trend([
    event('app_error', 'total', [], '报告次数'), event('app_error', 'dau', [], '受影响人数'),
  ], { properties: releaseProperties })),
  insight('errors', '14 · 应用报错分类', '正式版本号样本；前端异常、服务端 5xx/panic、API 4xx 分开。按版本、系统、固定操作和错误码定位，次数与受影响实例同时展示；旧版本 operation=legacy_unknown。无内容、路径、原始错误或堆栈。', sql(`
SELECT multiIf(properties.error_source = 'frontend', 'frontend',
         properties.error_source = 'backend' OR toInt(properties.status_code) >= 500, 'server', 'api_4xx') AS category,
       properties.app_version AS version, properties.os AS os,
       coalesce(nullIf(properties.operation, ''), 'legacy_unknown') AS operation,
       properties.error_source AS source, properties.error_code AS error_code,
       properties.status_code AS http_status, properties.feature_name AS feature,
       count() AS reports, uniqExact(distinct_id) AS affected_instances
FROM events WHERE {filters} AND ${releaseSql} AND event = 'app_error'
GROUP BY category, version, os, operation, source, error_code, http_status, feature ORDER BY category, reports DESC LIMIT 200`)),
  insight('run-errors', '15 · 执行失败原因', '正式版本号的用户执行失败，按版本、系统、功能、运行时及固定 error_code 分类。与 HTTP/前端报错分别统计；unknown 占比见接入状态，历史 unknown 无法反推根因。', sql(`
SELECT properties.app_version AS version, properties.os AS os,
       properties.feature_name AS feature, properties.runtime AS runtime, properties.error_code AS error_code,
       count() AS failed_runs, uniqExact(distinct_id) AS affected_instances
FROM events WHERE {filters} AND ${releaseSql} AND ${userSql} AND event = 'feature_failed'
GROUP BY version, os, feature, runtime, error_code ORDER BY failed_runs DESC LIMIT 200`)),
  insight('health', '16 · 数据接入状态', 'release_version=正式版本号格式，fork_or_development=其他版本；格式不能证明构建来源。两类样本分别保留，显示错误次数、受影响实例及执行失败 unknown 占比。旧事件不会因升级自动补齐原因；无失败样本时占比为空。', sql(`
SELECT if(${releaseVersionSql}, 'release_version', 'fork_or_development') AS sample,
       properties.app_version AS version, properties.os AS os,
       count() AS received_events, uniqExact(distinct_id) AS observed_instances,
       countIf(event = 'app_error') AS app_errors,
       uniqExactIf(distinct_id, event = 'app_error') AS affected_instances,
       countIf(event = 'feature_failed' AND ${userSql}) AS failed_runs,
       countIf(event = 'feature_failed' AND ${userSql} AND coalesce(nullIf(properties.error_code, ''), 'unknown') = 'unknown') AS unknown_runs,
       round(100.0 * unknown_runs / nullIf(failed_runs, 0), 2) AS unknown_pct,
       if(count() = 0, NULL, max(timestamp)) AS last_received_event
FROM events WHERE {filters} AND ${prodSql}
GROUP BY sample, version, os ORDER BY sample DESC, version DESC, os`)),
]

// Emit reviewable definitions for the authenticated UI when management credentials
// are unavailable. This mode neither contacts PostHog nor writes capture events.
if (process.argv.includes('--plan')) {
  console.log(JSON.stringify(insights, null, 2))
  process.exit(0)
}
if (!key?.startsWith('phx_')) throw new Error('Set POSTHOG_PERSONAL_API_KEY in the ignored .env file')

const project = await api(base)
const captureSource = await fs.readFile(new URL('internal/analytics/project.go', root), 'utf8')
assert.equal(project.api_token, captureSource.match(/const projectToken = "([^"]+)"/)[1], 'App token must match this project')
assert.equal(project.timezone, 'UTC', 'Daily App markers require the UTC project timezone')
console.log(JSON.stringify({ project: projectId, timezone: project.timezone, mode: apply ? 'apply' : 'verify' }))

// Query-only fixtures do not write synthetic users or events to the project.
const fixture = await query(sql(`
WITH sample AS (
 SELECT 'a' AS instance, 'success' AS outcome, 1000 AS duration_ms UNION ALL
 SELECT 'a', 'failure', 3000 UNION ALL SELECT 'b', 'cancelled', 5000
)
SELECT uniqExact(instance) AS instances, count() AS uses,
  round(100.0 * countIf(outcome = 'success') / nullIf(countIf(outcome IN ('success', 'failure')), 0), 2) AS success_pct,
  quantile(0.5)(duration_ms) AS p50_ms,
  1.0 / nullIf(countIf(outcome = 'absent'), 0) AS no_denominator
FROM sample`))
assert.deepEqual(fixture.results, [[2, 3, 50, 3000, null]])
console.log('PASS query-only fixture: distinct instances, event counts, success denominator, P50, null empty denominator')
const retentionFixture = await query(sql(retentionSummary(`
SELECT 'a' AS instance, 'writing' AS feature, toDate(now()) - INTERVAL 8 DAY AS cohort_day UNION ALL
SELECT 'b', 'writing', toDate(now()) - INTERVAL 2 DAY UNION ALL
SELECT 'c', 'writing', toDate(now()) - INTERVAL 8 DAY UNION ALL
SELECT 'd', 'writing', toDate(now()) - INTERVAL 1 DAY`, `
SELECT 'a' AS instance, 'writing' AS feature, [toDate(now()) - INTERVAL 7 DAY, toDate(now()) - INTERVAL 1 DAY] AS days UNION ALL
SELECT 'b', 'writing', [toDate(now()) - INTERVAL 1 DAY]`)))
assert.deepEqual(retentionFixture.results, [['writing', 3, 2, 66.67, 2, 1, 50, 0, 0, null]])
console.log('PASS query-only retention fixture: exact D1/D7/D30, mature denominators, missing returns, empty D30')

// Validate every source before saving any cloud objects.
for (const item of insights) {
  const result = await query(item.query.source)
  assert.ok(Array.isArray(result.results), `${item.slug}: query did not return results`)
  console.log(JSON.stringify({ validated: item.slug, rows: result.results.length }))
}

const dashboards = await api(base + 'dashboards/?limit=100')
let dashboard = dashboards.results.find(item => item.name === dashboardName && item.tags?.includes(tag))
if (!dashboard && !apply) throw new Error('Managed dashboard is missing; run with --apply to create it')
if (!dashboard) dashboard = await api(base + 'dashboards/', {
  name: dashboardName, pinned: true, tags: [tag],
  description: 'UTC · 匿名实例数 · schema 2 · 排除测试与自动任务。活跃/功能使用、写作与游戏留存、成功率、P50/P95 耗时、报错。无事件时请先确认新版 App 已启用分析；数据是尽力上报的观测样本。留存默认近 90 天，其他默认近 30 天；可在看板调整时间。',
})
let details = await api(base + `dashboards/${dashboard.id}/`)
const saved = []
for (const [order, item] of insights.entries()) {
  const itemTag = `${tag}:${item.slug}`
  let existing = details.tiles.find(tile => tile.insight?.tags?.includes(itemTag))?.insight
  // Recover an insight created immediately before an interrupted dashboard update.
  if (!existing) {
    const matches = await api(base + `insights/?search=${encodeURIComponent(item.name)}&limit=100`)
    existing = matches.results.find(candidate => candidate.tags?.includes(itemTag))
  }
  const payload = { name: item.name, description: item.description, query: item.query, order,
    dashboards: [...new Set([...(existing?.dashboards || []), dashboard.id])], tags: [tag, itemTag] }
  if (apply) existing = await api(base + `insights/${existing ? `${existing.id}/` : ''}`, payload, existing ? 'PATCH' : 'POST')
  if (!existing) throw new Error(`Missing managed insight: ${item.slug}`)
  const actual = await api(base + `insights/${existing.id}/`)
  assert.deepEqual(actual.query, item.query, `Saved query differs: ${item.slug}`)
  assert.equal(actual.description, item.description)
  assert.ok(actual.dashboards.includes(dashboard.id))
  saved.push({ slug: item.slug, id: actual.id, short_id: actual.short_id, name: actual.name })
  console.log(JSON.stringify({ saved: item.slug, id: actual.id }))
}
details = await api(base + `dashboards/${dashboard.id}/`)
assert.equal(saved.filter(item => details.tiles.some(tile => tile.insight?.id === item.id)).length, insights.length)
const sections = [
  { before: 'active', body: '## 活跃与功能使用\n\n按 UTC 统计匿名实例；同一人多台设备可能算多个实例。排除自动任务、诊断和测试事件。无正式事件时，图表为 0 或空白不代表没有用户；请先让用户升级并同意新版分析说明。\n\n人数 = 去重实例数；次数 = 有效使用里程碑；执行耗时包含完整过程。所有事件均为尽力上报的观测样本。' },
  { before: 'writing-retention', body: '## 激活与留存\n\n写作与游戏分开，排除旧用户迁移。留存默认近 90 天；D1 / D7 / D30 汇总只纳入已完整度过相应日期的队列。空分母不显示 0%。' },
  { before: 'success', body: '## 执行质量与报错\n\n成功率排除取消、暂停和抢占。P50 / P95 耗时请结合样本数看；完整执行队列用 run_id 连接，未结束或缺失事件单列。应用报错与执行失败可能来自同一事故，请分别查看。' },
]
for (const section of sections) {
  const marker = `<!-- ${tag}:${section.before} -->`
  let tile = details.tiles.find(candidate => candidate.text?.body?.includes(marker))
  if (!tile && apply) tile = await api(base + `dashboards/${dashboard.id}/create_text_tile/`, { body: section.body + '\n\n' + marker })
  if (!tile) throw new Error(`Missing dashboard section: ${section.before}`)
  section.id = tile.id
}
details = await api(base + `dashboards/${dashboard.id}/`)
let row = 0
const placements = []
for (const item of saved) {
  const section = sections.find(section => section.before === item.slug)
  if (section) {
    const height = item.slug === 'active' ? 3 : 2
    placements.push({ id: section.id, layouts: { sm: { x: 0, y: row, w: 12, h: height } } })
    row += height
  }
  const tile = details.tiles.find(tile => tile.insight?.id === item.id)
  const height = ['writing-retention', 'game-retention'].includes(item.slug) ? 7 : 5
  placements.push({ id: tile.id, layouts: { sm: { x: 0, y: row, w: 12, h: height } } })
  row += height
}
// PostHog derives the mobile xs layout from the saved 12-column sm layout.
if (apply) await api(base + `dashboards/${dashboard.id}/`, { tiles: placements }, 'PATCH')
details = await api(base + `dashboards/${dashboard.id}/`)
for (const placement of placements) {
  assert.deepEqual(details.tiles.find(tile => tile.id === placement.id)?.layouts, placement.layouts, `Saved layout differs: ${placement.id}`)
}
assert.equal(details.is_shared, false, 'The dashboard must remain private')
const url = `${origin}/project/${projectId}/dashboard/${dashboard.id}`
console.log(JSON.stringify({ dashboard: dashboard.id, url, verifiedInsights: saved.length }))
