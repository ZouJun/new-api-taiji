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
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { AlertCircle } from 'lucide-react'
import { FormDescription, FormLabel } from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { cn } from '@/lib/utils'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'
import {
  buildStrategyRowSignature,
  DEFAULT_TIMEOUT_MESSAGE,
  DEFAULT_TIMEOUT_STATUS,
  parseStrategyGroupNames,
  parseStrategyRows,
  serializeStrategyRows,
  type StrategyRow,
  type StrategyRowErrors,
  updateStrategyEnabled,
  updateStrategyNumberField,
  updateStrategyTextField,
  validateStrategyRows,
} from './strategy-settings-utils'

type StrategySettingsSectionProps = {
  groupRatio: string
  strategySettings: string
  globalRetryTimes: number
}

export function StrategySettingsSection({
  groupRatio,
  strategySettings,
  globalRetryTimes,
}: StrategySettingsSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()

  const groupNames = useMemo(
    () => parseStrategyGroupNames(groupRatio),
    [groupRatio]
  )
  const sourceRows = useMemo(
    () => parseStrategyRows(groupNames, strategySettings),
    [groupNames, strategySettings]
  )
  const sourceSignature = useMemo(
    () => buildStrategyRowSignature(sourceRows),
    [sourceRows]
  )

  const [rows, setRows] = useState<StrategyRow[]>(sourceRows)
  const [errors, setErrors] = useState<Record<string, StrategyRowErrors>>({})

  useEffect(() => {
    setRows(sourceRows)
    setErrors({})
  }, [sourceSignature, sourceRows])

  const currentSignature = useMemo(
    () => buildStrategyRowSignature(rows),
    [rows]
  )
  const hasChanges = currentSignature !== sourceSignature
  const hasGroups = groupNames.length > 0

  const onReset = () => {
    setRows(sourceRows)
    setErrors({})
  }

  const onSave = async () => {
    const nextErrors = validateStrategyRows(rows, t)
    setErrors(nextErrors)
    if (Object.keys(nextErrors).length > 0) {
      return
    }
    await updateOption.mutateAsync({
      key: 'group_strategy_settings',
      value: serializeStrategyRows(rows),
    })
  }

  return (
    <SettingsSection title={t('Strategy Settings')}>
      <div className='space-y-6'>
        <div className='space-y-4'>
          <p className='text-sm leading-6 text-muted-foreground'>
            {t(
              'Configure retry behavior and streaming first-byte retry budgets for each group.'
            )}
          </p>

          <div className='rounded-lg border border-border/70 bg-muted/20 p-4 text-sm'>
            <div className='flex items-start gap-3'>
              <AlertCircle className='mt-0.5 size-4 shrink-0 text-muted-foreground' />
              <div className='space-y-2 text-muted-foreground'>
                <p>
                  {t(
                    'When a group strategy is enabled, its retry count overrides global Retry Times. If no strategy is configured for a group, the system uses the current global retry and timeout behavior.'
                  )}
                </p>
                <p>
                  {t(
                    'Relay retry count controls cross-channel retries in New API. AWS SDK retry is separate. Per-channel stream timeout still limits each individual attempt, while the stream retry budget measures the total first-byte wait across retries.'
                  )}
                </p>
              </div>
            </div>
          </div>

          <div className='rounded-lg border border-border/70 p-4 text-sm'>
            <div className='space-y-2'>
              <p className='font-medium'>{t('Suggested usage scenarios')}</p>
              <ul className='space-y-2 text-muted-foreground'>
                <li>
                  {t(
                    'Example: Keep default conservative with 1 extra retry so ordinary traffic fails fast during upstream congestion.'
                  )}
                </li>
                <li>
                  {t(
                    'Example: Give vip a longer first-byte retry budget when premium users can wait a little longer for streaming channels.'
                  )}
                </li>
                <li>
                  {t(
                    'Example: Leave a group on fallback when it should follow the same policy as the global Retry Times and timeout defaults.'
                  )}
                </li>
              </ul>
            </div>
          </div>
        </div>

        <SettingsPageFormActions
          onSave={onSave}
          onReset={onReset}
          isSaving={updateOption.isPending}
          isSaveDisabled={!hasChanges || !hasGroups}
          isResetDisabled={!hasChanges}
          saveLabel='Save strategy settings'
        />

        {!hasGroups ? (
          <div className='rounded-lg border border-dashed border-border/70 p-6'>
            <div className='space-y-1'>
              <h3 className='text-sm font-semibold'>{t('No groups available')}</h3>
              <p className='text-sm text-muted-foreground'>
                {t(
                  'Create group ratios first, then return here to configure strategy behavior.'
                )}
              </p>
            </div>
          </div>
        ) : (
          <div className='space-y-4'>
            {rows.map((row) => {
              const rowErrors = errors[row.group] ?? {}
              const isFallback = !row.enabled
              return (
                <section
                  key={row.group}
                  className='rounded-lg border border-border/70 p-4'
                >
                  <div className='flex flex-col gap-4'>
                    <div className='flex flex-col gap-3 md:flex-row md:items-start md:justify-between'>
                      <div className='space-y-2'>
                        <div className='flex flex-wrap items-center gap-2'>
                          <h3 className='text-sm font-semibold'>{row.group}</h3>
                          <span
                            className={cn(
                              'rounded-md border px-2 py-0.5 text-xs',
                              isFallback
                                ? 'border-border/70 text-muted-foreground'
                                : 'border-primary/30 text-foreground'
                            )}
                          >
                            {isFallback
                              ? t('Uses global retry and timeout behavior')
                              : t('Custom strategy enabled')}
                          </span>
                        </div>
                        <p className='text-sm text-muted-foreground'>
                          {isFallback
                            ? t(
                                'This group currently follows the global Retry Times and existing timeout behavior.'
                              )
                            : t(
                                'This group uses its own relay retry ceiling and optional stream first-byte retry budget.'
                              )}
                        </p>
                      </div>

                      <div className='flex items-center justify-between gap-3 rounded-lg border border-border/70 px-3 py-2 md:min-w-56'>
                        <div className='space-y-0.5'>
                          <FormLabel>{t('Enable group strategy')}</FormLabel>
                          <FormDescription>
                            {t(
                              'Turn on per-group overrides. Turn it off to roll back to global behavior.'
                            )}
                          </FormDescription>
                        </div>
                        <Switch
                          checked={row.enabled}
                          onCheckedChange={(checked) => {
                            setRows((current) =>
                              updateStrategyEnabled(current, row.group, checked)
                            )
                            setErrors((current) => {
                              const next = { ...current }
                              delete next[row.group]
                              return next
                            })
                          }}
                        />
                      </div>
                    </div>

                    <div
                      className={cn(
                        'grid gap-4 lg:grid-cols-2',
                        !row.enabled && 'opacity-70'
                      )}
                    >
                      <div className='space-y-2'>
                        <FormLabel>{t('Stream retry first-byte budget (seconds)')}</FormLabel>
                        <Input
                          type='number'
                          min='1'
                          value={row.streamRetryFirstByteBudgetSeconds}
                          onChange={(event) =>
                            setRows((current) =>
                              updateStrategyNumberField(
                                current,
                                row.group,
                                'streamRetryFirstByteBudgetSeconds',
                                event.target.value
                              )
                            )
                          }
                        />
                        <FormDescription>
                          {t(
                            'Maximum total seconds spent waiting for the first stream response across retries for this group.'
                          )}
                        </FormDescription>
                        <p className='text-xs text-muted-foreground'>
                          {row.streamRetryFirstByteBudgetSeconds === ''
                            ? t(
                                'Blank uses per-channel and global timeout settings only. This budget counts retry wait for the first stream token, not the full stream duration.'
                              )
                            : t(
                                'Current budget: {{seconds}} seconds of total retry first-byte waiting.',
                                {
                                  seconds: row.streamRetryFirstByteBudgetSeconds,
                                }
                              )}
                        </p>
                        {rowErrors.streamRetryFirstByteBudgetSeconds ? (
                          <p className='text-destructive text-xs'>
                            {rowErrors.streamRetryFirstByteBudgetSeconds}
                          </p>
                        ) : null}
                      </div>

                      <div className='space-y-2'>
                        <FormLabel>{t('Retry times')}</FormLabel>
                        <Input
                          type='number'
                          min='0'
                          value={row.retryTimes}
                          onChange={(event) =>
                            setRows((current) =>
                              updateStrategyNumberField(
                                current,
                                row.group,
                                'retryTimes',
                                event.target.value
                              )
                            )
                          }
                        />
                        <FormDescription>
                          {t(
                            'Additional relay retries for this group. The first request is not counted.'
                          )}
                        </FormDescription>
                        <p className='text-xs text-muted-foreground'>
                          {row.retryTimes === ''
                            ? t(
                                'Blank falls back to global Retry Times: {{count}}. This only affects New API channel switching, not AWS SDK retries.',
                                {
                                  count: globalRetryTimes,
                                }
                              )
                            : t(
                                'Current setting allows {{count}} extra retries after the first request.',
                                {
                                  count: row.retryTimes,
                                }
                              )}
                        </p>
                        {rowErrors.retryTimes ? (
                          <p className='text-destructive text-xs'>
                            {rowErrors.retryTimes}
                          </p>
                        ) : null}
                      </div>

                      <div className='space-y-2'>
                        <FormLabel>
                          {t(
                            'HTTP status returned when stream first-byte wait budget is exhausted'
                          )}
                        </FormLabel>
                        <Input
                          type='number'
                          min='100'
                          max='599'
                          value={row.timeoutHTTPStatus}
                          onChange={(event) =>
                            setRows((current) =>
                              updateStrategyNumberField(
                                current,
                                row.group,
                                'timeoutHTTPStatus',
                                event.target.value
                              )
                            )
                          }
                        />
                        <FormDescription>
                          {t(
                            'Returned to the customer only when this group ends because the total stream first-byte wait budget is exhausted.'
                          )}
                        </FormDescription>
                        <p className='text-xs text-muted-foreground'>
                          {row.timeoutHTTPStatus === ''
                            ? t('Blank falls back to HTTP status {{status}}.', {
                                status: DEFAULT_TIMEOUT_STATUS,
                              })
                            : t(
                                'Current response status for stream first-byte budget exhaustion: {{status}}.',
                                {
                                status: row.timeoutHTTPStatus,
                                }
                              )}
                        </p>
                        {rowErrors.timeoutHTTPStatus ? (
                          <p className='text-destructive text-xs'>
                            {rowErrors.timeoutHTTPStatus}
                          </p>
                        ) : null}
                      </div>

                      <div className='space-y-2'>
                        <FormLabel>
                          {t(
                            'Error message returned when stream first-byte wait budget is exhausted'
                          )}
                        </FormLabel>
                        <Input
                          value={row.timeoutErrorMessage}
                          onChange={(event) =>
                            setRows((current) =>
                              updateStrategyTextField(
                                current,
                                row.group,
                                event.target.value
                              )
                            )
                          }
                        />
                        <FormDescription>
                          {t(
                            'Returned to the customer only when the total stream first-byte wait budget is exhausted. Leave empty to use: 资源繁忙，请稍后尝试'
                          )}
                        </FormDescription>
                        <p className='text-xs text-muted-foreground'>
                          {row.timeoutErrorMessage.trim()
                            ? t(
                                'Current error message for stream first-byte budget exhaustion: {{message}}',
                                {
                                message: row.timeoutErrorMessage.trim(),
                                }
                              )
                            : t('Blank falls back to: {{message}}', {
                                message: DEFAULT_TIMEOUT_MESSAGE,
                              })}
                        </p>
                      </div>
                    </div>
                  </div>
                </section>
              )
            })}
          </div>
        )}
      </div>
    </SettingsSection>
  )
}
