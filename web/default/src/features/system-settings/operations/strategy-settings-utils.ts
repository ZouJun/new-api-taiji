/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { safeJsonParse } from '../utils/json-parser'

export type StrategyOption = {
  enabled?: boolean
  stream_retry_first_byte_budget_seconds?: number
  retry_times?: number
  timeout_http_status?: number
  timeout_error_message?: string
}

export type StrategyRow = {
  group: string
  enabled: boolean
  streamRetryFirstByteBudgetSeconds: number | ''
  retryTimes: number | ''
  timeoutHTTPStatus: number | ''
  timeoutErrorMessage: string
  existsInOption: boolean
}

export type StrategyRowField =
  | 'streamRetryFirstByteBudgetSeconds'
  | 'retryTimes'
  | 'timeoutHTTPStatus'

export type StrategyRowErrors = Partial<Record<StrategyRowField, string>>

export const DEFAULT_TIMEOUT_STATUS = 503
export const DEFAULT_TIMEOUT_MESSAGE = '资源繁忙，请稍后尝试'

export function parseStrategyGroupNames(groupRatio: string) {
  const map = safeJsonParse<Record<string, number>>(groupRatio, {
    fallback: {},
    context: 'group ratios',
  })
  return Object.keys(map).sort((a, b) => a.localeCompare(b))
}

export function parseStrategyRows(
  groupNames: string[],
  strategySettings: string
): StrategyRow[] {
  const stored = safeJsonParse<Record<string, StrategyOption>>(strategySettings, {
    fallback: {},
    context: 'group strategy settings',
  })

  return groupNames.map((group) => {
    const current = stored[group]
    return {
      group,
      enabled: Boolean(current?.enabled),
      streamRetryFirstByteBudgetSeconds:
        typeof current?.stream_retry_first_byte_budget_seconds === 'number'
          ? current.stream_retry_first_byte_budget_seconds
          : '',
      retryTimes:
        typeof current?.retry_times === 'number' ? current.retry_times : '',
      timeoutHTTPStatus:
        typeof current?.timeout_http_status === 'number'
          ? current.timeout_http_status
          : '',
      timeoutErrorMessage: current?.timeout_error_message ?? '',
      existsInOption: Boolean(current),
    }
  })
}

export function serializeStrategyRows(rows: StrategyRow[]) {
  const payload: Record<string, StrategyOption> = {}

  for (const row of rows) {
    const item: StrategyOption = {
      enabled: row.enabled,
    }
    if (row.streamRetryFirstByteBudgetSeconds !== '') {
      item.stream_retry_first_byte_budget_seconds =
        row.streamRetryFirstByteBudgetSeconds
    }
    if (row.retryTimes !== '') {
      item.retry_times = row.retryTimes
    }
    if (row.timeoutHTTPStatus !== '') {
      item.timeout_http_status = row.timeoutHTTPStatus
    }
    if (row.timeoutErrorMessage.trim()) {
      item.timeout_error_message = row.timeoutErrorMessage.trim()
    }

    const hasCustomField =
      item.stream_retry_first_byte_budget_seconds !== undefined ||
      item.retry_times !== undefined ||
      item.timeout_http_status !== undefined ||
      Boolean(item.timeout_error_message)

    if (row.enabled || hasCustomField || row.existsInOption) {
      payload[row.group] = item
    }
  }

  return JSON.stringify(payload, null, 2)
}

export function buildStrategyRowSignature(rows: StrategyRow[]) {
  return JSON.stringify(
    rows.map((row) => ({
      group: row.group,
      enabled: row.enabled,
      streamRetryFirstByteBudgetSeconds: row.streamRetryFirstByteBudgetSeconds,
      retryTimes: row.retryTimes,
      timeoutHTTPStatus: row.timeoutHTTPStatus,
      timeoutErrorMessage: row.timeoutErrorMessage,
      existsInOption: row.existsInOption,
    }))
  )
}

export function validateStrategyRows(
  rows: StrategyRow[],
  t: (key: string) => string
) {
  const errors: Record<string, StrategyRowErrors> = {}

  for (const row of rows) {
    if (!row.enabled) continue
    const rowErrors: StrategyRowErrors = {}

    if (
      row.streamRetryFirstByteBudgetSeconds !== '' &&
      row.streamRetryFirstByteBudgetSeconds < 1
    ) {
      rowErrors.streamRetryFirstByteBudgetSeconds = t(
        'Retry budget must be at least 1 second'
      )
    }
    if (row.retryTimes !== '' && row.retryTimes < 0) {
      rowErrors.retryTimes = t('Retry times must be 0 or greater')
    }
    if (
      row.timeoutHTTPStatus !== '' &&
      (row.timeoutHTTPStatus < 100 || row.timeoutHTTPStatus > 599)
    ) {
      rowErrors.timeoutHTTPStatus = t(
        'HTTP status must be between 100 and 599'
      )
    }
    if (Object.keys(rowErrors).length > 0) {
      errors[row.group] = rowErrors
    }
  }

  return errors
}

export function updateStrategyNumberField(
  rows: StrategyRow[],
  group: string,
  field: StrategyRowField,
  value: string
) {
  return rows.map((row) => {
    if (row.group !== group) return row
    if (value.trim() === '') {
      return {
        ...row,
        [field]: '',
      }
    }
    return {
      ...row,
      [field]: Number(value),
    }
  })
}

export function updateStrategyTextField(
  rows: StrategyRow[],
  group: string,
  value: string
) {
  return rows.map((row) =>
    row.group === group ? { ...row, timeoutErrorMessage: value } : row
  )
}

export function updateStrategyEnabled(
  rows: StrategyRow[],
  group: string,
  enabled: boolean
) {
  return rows.map((row) =>
    row.group === group ? { ...row, enabled } : row
  )
}
