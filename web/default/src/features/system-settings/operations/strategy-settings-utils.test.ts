import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import {
  buildStrategyRowSignature,
  parseStrategyGroupNames,
  parseStrategyRows,
  serializeStrategyRows,
  updateStrategyEnabled,
  updateStrategyNumberField,
  updateStrategyTextField,
  validateStrategyRows,
  type StrategyRow,
} from './strategy-settings-utils'

function makeRows(): StrategyRow[] {
  return [
    {
      group: 'default',
      enabled: false,
      streamRetryFirstByteBudgetSeconds: '',
      retryTimes: '',
      timeoutHTTPStatus: '',
      timeoutErrorMessage: '',
      existsInOption: false,
    },
    {
      group: 'vip',
      enabled: true,
      streamRetryFirstByteBudgetSeconds: 30,
      retryTimes: 2,
      timeoutHTTPStatus: 429,
      timeoutErrorMessage: 'busy',
      existsInOption: true,
    },
  ]
}

describe('strategy-settings-utils', () => {
  test('parses group names in stable sorted order', () => {
    assert.deepEqual(
      parseStrategyGroupNames('{"vip":1,"default":1,"beta":1}'),
      ['beta', 'default', 'vip']
    )
  })

  test('parses rows with fallback blanks for missing strategy fields', () => {
    const rows = parseStrategyRows(
      ['default', 'vip'],
      '{"vip":{"enabled":true,"retry_times":2}}'
    )

    assert.deepEqual(rows, [
      {
        group: 'default',
        enabled: false,
        streamRetryFirstByteBudgetSeconds: '',
        retryTimes: '',
        timeoutHTTPStatus: '',
        timeoutErrorMessage: '',
        existsInOption: false,
      },
      {
        group: 'vip',
        enabled: true,
        streamRetryFirstByteBudgetSeconds: '',
        retryTimes: 2,
        timeoutHTTPStatus: '',
        timeoutErrorMessage: '',
        existsInOption: true,
      },
    ])
  })

  test('serializes enabled rows and keeps disabled existing rows for auditability', () => {
    const serialized = serializeStrategyRows([
      {
        group: 'default',
        enabled: false,
        streamRetryFirstByteBudgetSeconds: '',
        retryTimes: '',
        timeoutHTTPStatus: '',
        timeoutErrorMessage: '',
        existsInOption: true,
      },
      {
        group: 'vip',
        enabled: true,
        streamRetryFirstByteBudgetSeconds: 30,
        retryTimes: 2,
        timeoutHTTPStatus: 429,
        timeoutErrorMessage: ' busy ',
        existsInOption: true,
      },
    ])

    assert.deepEqual(JSON.parse(serialized), {
      default: {
        enabled: false,
      },
      vip: {
        enabled: true,
        stream_retry_first_byte_budget_seconds: 30,
        retry_times: 2,
        timeout_http_status: 429,
        timeout_error_message: 'busy',
      },
    })
  })

  test('validates only enabled rows', () => {
    const errors = validateStrategyRows(
      [
        {
          group: 'default',
          enabled: false,
          streamRetryFirstByteBudgetSeconds: 0,
          retryTimes: -1,
          timeoutHTTPStatus: 99,
          timeoutErrorMessage: '',
          existsInOption: true,
        },
        {
          group: 'vip',
          enabled: true,
          streamRetryFirstByteBudgetSeconds: 0,
          retryTimes: -1,
          timeoutHTTPStatus: 600,
          timeoutErrorMessage: '',
          existsInOption: true,
        },
      ],
      (key) => key
    )

    assert.deepEqual(errors, {
      vip: {
        streamRetryFirstByteBudgetSeconds:
          'Retry budget must be at least 1 second',
        retryTimes: 'Retry times must be 0 or greater',
        timeoutHTTPStatus: 'HTTP status must be between 100 and 599',
      },
    })
  })

  test('updates row fields without mutating other groups', () => {
    const rows = makeRows()
    const updatedNumbers = updateStrategyNumberField(
      rows,
      'default',
      'retryTimes',
      '3'
    )
    const updatedText = updateStrategyTextField(
      updatedNumbers,
      'default',
      'fallback message'
    )
    const updatedEnabled = updateStrategyEnabled(updatedText, 'default', true)

    assert.equal(updatedEnabled[0].retryTimes, 3)
    assert.equal(updatedEnabled[0].timeoutErrorMessage, 'fallback message')
    assert.equal(updatedEnabled[0].enabled, true)
    assert.equal(updatedEnabled[1].retryTimes, 2)
    assert.equal(updatedEnabled[1].timeoutErrorMessage, 'busy')
  })

  test('builds different signatures when row values change', () => {
    const rows = makeRows()
    const original = buildStrategyRowSignature(rows)
    const updated = buildStrategyRowSignature(
      updateStrategyEnabled(rows, 'default', true)
    )

    assert.notEqual(original, updated)
  })
})
