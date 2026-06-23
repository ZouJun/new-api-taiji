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
import * as z from 'zod'
import { useMemo } from 'react'
import { type Resolver, useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import {
  SettingsControlGroup,
  SettingsForm,
  SettingsFormGrid,
  SettingsFormGridItem,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsSection } from '../components/settings-section'
import { updateSystemOptionsBulk } from '../api'
import { useResetForm } from '../hooks/use-reset-form'
import type { OperationsSettings } from '../types'
import { safeNumberFieldProps } from '../utils/numeric-field'

const createSchema = (t: (key: string) => string) =>
  z
    .object({
      ArchiveEnabled: z.boolean(),
      ArchiveBackend: z.enum(['local', 'azure_blob']),
      ArchiveLocalDir: z.string(),
      ArchiveSpoolDir: z.string(),
      ArchiveQueueSize: z.coerce.number().int().min(1).max(1000000),
      ArchiveWorkerCount: z.coerce.number().int().min(1).max(1024),
      ArchiveMaxRequestMB: z.coerce.number().int().min(1).max(10240),
      ArchiveMaxResponseMB: z.coerce.number().int().min(1).max(10240),
      ArchiveSpoolTTLHours: z.coerce.number().int().min(1).max(24 * 365),
      ArchiveSmallPayloadMaxKB: z.coerce.number().int().min(1).max(1024 * 1024),
      ArchiveSegmentMaxMB: z.coerce.number().int().min(1).max(10240),
      ArchiveSegmentMaxAgeSeconds: z.coerce.number().int().min(1).max(86400),
      ArchiveSegmentMaxRecords: z.coerce.number().int().min(1).max(1000000),
      ArchiveSegmentShardCount: z.coerce.number().int().min(1).max(4096),
      ArchiveHeaderValueMaxLength: z.coerce.number().int().min(0).max(65535),
      ArchiveSamplePercent: z.coerce.number().int().min(0).max(100),
      ArchiveSkipOnHighLoad: z.boolean(),
      ArchiveMaxCPUPercent: z.coerce.number().int().min(0).max(100),
      ArchiveMaxMemoryPercent: z.coerce.number().int().min(0).max(100),
      ArchiveMinFreeDiskPercent: z.coerce.number().int().min(0).max(100),
      ArchiveMinFreeDiskGB: z.coerce.number().int().min(0).max(1048576),
      ArchiveLoadCheckIntervalSeconds: z.coerce.number().int().min(1).max(3600),
      ArchiveAzureAccountURL: z.string(),
      ArchiveAzureContainer: z.string(),
      ArchiveAzureAccountName: z.string(),
      ArchiveAzureAccountKey: z.string(),
    })
    .superRefine((values, ctx) => {
      if (values.ArchiveBackend !== 'azure_blob') {
        return
      }
      if (values.ArchiveAzureAccountURL.trim() === '') {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          path: ['ArchiveAzureAccountURL'],
          message: t('Azure account URL is required when Azure Blob is enabled'),
        })
      }
      if (values.ArchiveAzureContainer.trim() === '') {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          path: ['ArchiveAzureContainer'],
          message: t('Azure container is required when Azure Blob is enabled'),
        })
      }
    })

type StorageFormValues = z.infer<ReturnType<typeof createSchema>>

type StorageSettingsSectionProps = {
  defaultValues: OperationsSettings
}

type FieldMeta = {
  label: string
  help: string
  recommendation: string
}

const storageKeys = [
  'ArchiveEnabled',
  'ArchiveBackend',
  'ArchiveLocalDir',
  'ArchiveSpoolDir',
  'ArchiveQueueSize',
  'ArchiveWorkerCount',
  'ArchiveMaxRequestMB',
  'ArchiveMaxResponseMB',
  'ArchiveSpoolTTLHours',
  'ArchiveSmallPayloadMaxKB',
  'ArchiveSegmentMaxMB',
  'ArchiveSegmentMaxAgeSeconds',
  'ArchiveSegmentMaxRecords',
  'ArchiveSegmentShardCount',
  'ArchiveHeaderValueMaxLength',
  'ArchiveSamplePercent',
  'ArchiveSkipOnHighLoad',
  'ArchiveMaxCPUPercent',
  'ArchiveMaxMemoryPercent',
  'ArchiveMinFreeDiskPercent',
  'ArchiveMinFreeDiskGB',
  'ArchiveLoadCheckIntervalSeconds',
  'ArchiveAzureAccountURL',
  'ArchiveAzureContainer',
  'ArchiveAzureAccountName',
  'ArchiveAzureAccountKey',
] as const satisfies ReadonlyArray<keyof StorageFormValues>

const storageRecommendedDefaults: StorageFormValues = {
  ArchiveEnabled: false,
  ArchiveBackend: 'local',
  ArchiveLocalDir: './data/archive',
  ArchiveSpoolDir: '',
  ArchiveQueueSize: 50000,
  ArchiveWorkerCount: 32,
  ArchiveMaxRequestMB: 128,
  ArchiveMaxResponseMB: 128,
  ArchiveSpoolTTLHours: 24,
  ArchiveSmallPayloadMaxKB: 64,
  ArchiveSegmentMaxMB: 256,
  ArchiveSegmentMaxAgeSeconds: 60,
  ArchiveSegmentMaxRecords: 50000,
  ArchiveSegmentShardCount: 16,
  ArchiveHeaderValueMaxLength: 512,
  ArchiveSamplePercent: 100,
  ArchiveSkipOnHighLoad: true,
  ArchiveMaxCPUPercent: 80,
  ArchiveMaxMemoryPercent: 80,
  ArchiveMinFreeDiskPercent: 15,
  ArchiveMinFreeDiskGB: 10,
  ArchiveLoadCheckIntervalSeconds: 5,
  ArchiveAzureAccountURL: '',
  ArchiveAzureContainer: '',
  ArchiveAzureAccountName: '',
  ArchiveAzureAccountKey: '',
}

const fieldMeta: Record<keyof StorageFormValues, FieldMeta> = {
  ArchiveEnabled: {
    label: 'Enable archive storage',
    help: 'Master switch for phase3 raw request and response archiving. When this is off, the relay path skips all archive capture logic.',
    recommendation:
      'Keep it off by default. Turn it on only when you need replay, audit, or upstream dispute evidence.',
  },
  ArchiveBackend: {
    label: 'Storage location',
    help: 'Choose where finalized archive objects land. Local keeps final objects on the current node. Azure Blob uploads finalized objects while still using local staging files during processing.',
    recommendation:
      'Use local for a single stable node. Use Azure Blob when you need centralized long-term retention.',
  },
  ArchiveLocalDir: {
    label: 'Archive root directory',
    help: 'Root directory for local archive objects and default staging subdirectories. This path must be on a persistent volume with predictable IOPS.',
    recommendation:
      'Recommended: a dedicated persistent SSD path such as ./data/archive or a mounted data volume.',
  },
  ArchiveSpoolDir: {
    label: 'Archive spool directory',
    help: 'Temporary spill directory used while request and response bodies are being staged before finalization. Leaving it empty lets the service create {localDir}/spool automatically.',
    recommendation:
      'Recommended: leave empty unless you intentionally separate hot staging IO from final object storage.',
  },
  ArchiveQueueSize: {
    label: 'Queue size',
    help: 'Maximum number of archive jobs buffered in memory before workers consume them. This protects the main relay from being blocked by archive IO, but a larger queue also means more buffered work.',
    recommendation:
      'Recommended: 20000 to 50000 for 8000-15000 RPM. Do not raise it blindly on memory-constrained nodes.',
  },
  ArchiveWorkerCount: {
    label: 'Worker count',
    help: 'Background workers that finalize archive jobs. More workers can improve throughput, but they also increase concurrent disk and network IO.',
    recommendation:
      'Recommended: 16 to 32 on dedicated nodes with SSD. Reduce to 8 to 16 on shared disks or weaker CPU.',
  },
  ArchiveMaxRequestMB: {
    label: 'Max request size (MB)',
    help: 'Hard cap for raw request body capture. Anything larger is skipped to avoid oversized archive jobs and extreme disk pressure.',
    recommendation:
      'Recommended: 64 to 128 MB. Only raise it if real business traffic requires it.',
  },
  ArchiveMaxResponseMB: {
    label: 'Max response size (MB)',
    help: 'Hard cap for raw upstream or downstream response capture. Anything larger is skipped to keep archive cost bounded.',
    recommendation:
      'Recommended: 64 to 128 MB. Keep it aligned with the largest real model outputs you must retain.',
  },
  ArchiveSpoolTTLHours: {
    label: 'Spool retention (hours)',
    help: 'How long temporary spool files may remain before cleanup treats them as stale. This covers crash recovery and interrupted finalization.',
    recommendation:
      'Recommended: 6 to 24 hours. Longer retention helps forensic recovery but increases disk occupancy.',
  },
  ArchiveSmallPayloadMaxKB: {
    label: 'Small payload max size (KB)',
    help: 'Payloads at or below this threshold may enter segmented aggregation mode. Payloads above it go through per-request object flow.',
    recommendation:
      'Recommended: 32 to 128 KB. Keep it near your real small-request cluster, not your largest payloads.',
  },
  ArchiveSegmentMaxMB: {
    label: 'Segment max size (MB)',
    help: 'Upper bound of one aggregated segment file before rotation. Larger segments reduce file counts but make retries and re-uploads heavier.',
    recommendation:
      'Recommended: 128 to 256 MB on SSD. Reduce to 64 to 128 MB on slower disks or busy nodes.',
  },
  ArchiveSegmentMaxAgeSeconds: {
    label: 'Segment max age (seconds)',
    help: 'Maximum open time for a segment before it rotates, even if size and record count are not yet reached.',
    recommendation:
      'Recommended: 30 to 60 seconds. Shorter intervals reduce recovery windows and ease correlation.',
  },
  ArchiveSegmentMaxRecords: {
    label: 'Segment max records',
    help: 'Maximum number of archived objects inside one segment before rotation. This limits segment fan-in and keeps lookup and upload units bounded.',
    recommendation:
      'Recommended: 10000 to 50000. Use the lower end if each record contains richer metadata.',
  },
  ArchiveSegmentShardCount: {
    label: 'Segment shard count',
    help: 'Number of shard streams for segmented writes. More shards reduce lock contention at high RPM, but too many small shards increase file churn.',
    recommendation:
      'Recommended: 8 to 16 for most production nodes. Increase only when write contention is proven.',
  },
  ArchiveHeaderValueMaxLength: {
    label: 'Header value max length',
    help: 'Maximum stored length for each tracked request header value before truncation. This bounds unexpected oversized header payloads.',
    recommendation:
      'Recommended: 256 to 512 characters. Larger values rarely improve traceability.',
  },
  ArchiveSamplePercent: {
    label: 'Sample percent',
    help: 'Probability gate for archive capture after feature enablement and load checks. 100 means all eligible requests are archived.',
    recommendation:
      'Recommended: 10 to 30 for high-RPM production if full retention is not mandatory. Use 100 only when complete evidence is required.',
  },
  ArchiveSkipOnHighLoad: {
    label: 'Skip archive on high load',
    help: 'When enabled, archive capture is bypassed once configured CPU, memory, or disk thresholds are exceeded. This protects the relay path first.',
    recommendation:
      'Recommended: keep enabled in production unless archive retention is more important than relay stability.',
  },
  ArchiveMaxCPUPercent: {
    label: 'CPU threshold (%)',
    help: 'If current CPU usage exceeds this threshold, archive capture is skipped while load shedding is enabled.',
    recommendation:
      'Recommended: 70 to 85 percent. Lower values are safer; higher values risk competing with the relay path.',
  },
  ArchiveMaxMemoryPercent: {
    label: 'Memory threshold (%)',
    help: 'If memory usage exceeds this threshold, archive capture is skipped while load shedding is enabled.',
    recommendation:
      'Recommended: 70 to 85 percent. Stay conservative on nodes with other co-located services.',
  },
  ArchiveMinFreeDiskPercent: {
    label: 'Minimum free disk (%)',
    help: 'Archive capture is skipped when free disk percentage falls below this threshold. This avoids archive writes consuming the last safe capacity.',
    recommendation:
      'Recommended: 10 to 20 percent. Increase it if disk growth is bursty or cleanup windows are slow.',
  },
  ArchiveMinFreeDiskGB: {
    label: 'Minimum free disk (GB)',
    help: 'Absolute free-disk floor. Archive capture is skipped if available space drops below this value, even when free percentage still looks healthy.',
    recommendation:
      'Recommended: 10 to 20 GB for small nodes. Raise it on large disks with heavy burst traffic.',
  },
  ArchiveLoadCheckIntervalSeconds: {
    label: 'Load check interval (seconds)',
    help: 'How often the service refreshes CPU, memory, and disk pressure state for archive gating.',
    recommendation:
      'Recommended: 5 to 15 seconds. Faster checks react sooner but sample the host more often.',
  },
  ArchiveAzureAccountURL: {
    label: 'Azure account URL',
    help: 'Blob service endpoint, optionally with SAS credentials. Example: https://account.blob.core.windows.net or a SAS-signed container URL.',
    recommendation:
      'Recommended: prefer SAS-scoped URLs when operators should not hold full account keys.',
  },
  ArchiveAzureContainer: {
    label: 'Azure container',
    help: 'Target container name for finalized archive objects.',
    recommendation:
      'Recommended: use a dedicated container for archive data so lifecycle and access policies stay isolated.',
  },
  ArchiveAzureAccountName: {
    label: 'Azure account name',
    help: 'Optional shared-key account identity. It is only needed when the account URL does not already carry SAS authorization.',
    recommendation:
      'Recommended: leave empty when using SAS-based URLs.',
  },
  ArchiveAzureAccountKey: {
    label: 'Azure account key',
    help: 'Optional shared key paired with account name. It is only needed when the account URL does not already carry SAS authorization.',
    recommendation:
      'Recommended: leave empty when using SAS-based URLs. Avoid long-lived full-account keys when possible.',
  },
}

function buildDefaults(settings: OperationsSettings): StorageFormValues {
  return {
    ArchiveEnabled: settings.ArchiveEnabled,
    ArchiveBackend:
      settings.ArchiveBackend === 'azure_blob' ? 'azure_blob' : 'local',
    ArchiveLocalDir: settings.ArchiveLocalDir ?? '',
    ArchiveSpoolDir: settings.ArchiveSpoolDir ?? '',
    ArchiveQueueSize: settings.ArchiveQueueSize,
    ArchiveWorkerCount: settings.ArchiveWorkerCount,
    ArchiveMaxRequestMB: settings.ArchiveMaxRequestMB,
    ArchiveMaxResponseMB: settings.ArchiveMaxResponseMB,
    ArchiveSpoolTTLHours: settings.ArchiveSpoolTTLHours,
    ArchiveSmallPayloadMaxKB: settings.ArchiveSmallPayloadMaxKB,
    ArchiveSegmentMaxMB: settings.ArchiveSegmentMaxMB,
    ArchiveSegmentMaxAgeSeconds: settings.ArchiveSegmentMaxAgeSeconds,
    ArchiveSegmentMaxRecords: settings.ArchiveSegmentMaxRecords,
    ArchiveSegmentShardCount: settings.ArchiveSegmentShardCount,
    ArchiveHeaderValueMaxLength: settings.ArchiveHeaderValueMaxLength,
    ArchiveSamplePercent: settings.ArchiveSamplePercent,
    ArchiveSkipOnHighLoad: settings.ArchiveSkipOnHighLoad,
    ArchiveMaxCPUPercent: settings.ArchiveMaxCPUPercent,
    ArchiveMaxMemoryPercent: settings.ArchiveMaxMemoryPercent,
    ArchiveMinFreeDiskPercent: settings.ArchiveMinFreeDiskPercent,
    ArchiveMinFreeDiskGB: settings.ArchiveMinFreeDiskGB,
    ArchiveLoadCheckIntervalSeconds: settings.ArchiveLoadCheckIntervalSeconds,
    ArchiveAzureAccountURL: settings.ArchiveAzureAccountURL ?? '',
    ArchiveAzureContainer: settings.ArchiveAzureContainer ?? '',
    ArchiveAzureAccountName: settings.ArchiveAzureAccountName ?? '',
    ArchiveAzureAccountKey: settings.ArchiveAzureAccountKey ?? '',
  }
}

function sanitizeValues(values: StorageFormValues): StorageFormValues {
  return {
    ...values,
    ArchiveLocalDir: values.ArchiveLocalDir.trim(),
    ArchiveSpoolDir: values.ArchiveSpoolDir.trim(),
    ArchiveAzureAccountURL: values.ArchiveAzureAccountURL.trim(),
    ArchiveAzureContainer: values.ArchiveAzureContainer.trim(),
    ArchiveAzureAccountName: values.ArchiveAzureAccountName.trim(),
    ArchiveAzureAccountKey: values.ArchiveAzureAccountKey.trim(),
  }
}

function getPlaceholderValue(key: keyof StorageFormValues): string {
  const recommended = storageRecommendedDefaults[key]
  if (typeof recommended === 'number') {
    return String(recommended)
  }
  if (typeof recommended === 'string' && recommended !== '') {
    return recommended
  }

  switch (key) {
    case 'ArchiveSpoolDir':
      return '{archive_root}/spool'
    case 'ArchiveAzureAccountURL':
      return 'https://<account>.blob.core.windows.net'
    case 'ArchiveAzureContainer':
      return 'archive'
    case 'ArchiveAzureAccountName':
      return 'storage account name'
    case 'ArchiveAzureAccountKey':
      return 'leave empty when using SAS URL'
    default:
      return ''
  }
}

function formatPlaceholder(
  t: (key: string) => string,
  key: keyof StorageFormValues
): string {
  const suggestion = getPlaceholderValue(key)
  return suggestion === '' ? '' : `${t('Suggested setting:')} ${t(suggestion)}`
}

function withRecommendedDefaults(
  values: StorageFormValues
): StorageFormValues {
  const next = { ...values }

  for (const key of storageKeys) {
    if (key === 'ArchiveEnabled') {
      continue
    }

    const current = next[key]
    const recommended = storageRecommendedDefaults[key]

    if (typeof recommended === 'number') {
      if (typeof current !== 'number' || current <= 0) {
        next[key] = recommended
      }
      continue
    }

    if (typeof recommended === 'string' && current.trim() === '') {
      next[key] = recommended
    }
  }

  return next
}

function FieldLabel({
  meta,
  t,
}: {
  meta: FieldMeta
  t: (key: string) => string
}) {
  return (
    <div className='flex flex-wrap items-center gap-x-2 gap-y-1 leading-5'>
      <span>{t(meta.label)}</span>
      <span className='text-muted-foreground text-[11px]'>
        {`${t('Suggested setting:')} ${t(meta.recommendation)}`}
      </span>
    </div>
  )
}

function GroupHeader({
  title,
  description,
}: {
  title: string
  description: string
}) {
  return (
    <div className='space-y-1'>
      <h3 className='text-sm font-medium'>{title}</h3>
      <p className='text-muted-foreground text-xs leading-5'>{description}</p>
    </div>
  )
}

export function StorageSettingsSection({
  defaultValues,
}: StorageSettingsSectionProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const schema = createSchema(t)
  const currentDefaults = useMemo(
    () => buildDefaults(defaultValues),
    [defaultValues]
  )
  const form = useForm<StorageFormValues>({
    resolver: zodResolver(schema) as Resolver<StorageFormValues>,
    defaultValues: currentDefaults,
  })
  const updateOptions = useMutation({
    mutationFn: updateSystemOptionsBulk,
    onSuccess: data => {
      if (!data.success) {
        toast.error(data.message || t('Failed to update setting'))
        return
      }
      form.reset(sanitizeValues(form.getValues()))
      queryClient.invalidateQueries({ queryKey: ['system-options'] })
      toast.success(t('Storage settings saved successfully'))
    },
    onError: (error: Error) => {
      toast.error(error.message || t('Failed to update setting'))
    },
  })

  useResetForm(form, currentDefaults)

  const archiveEnabled = form.watch('ArchiveEnabled')
  const backend = form.watch('ArchiveBackend')

  const onSubmit = async (values: StorageFormValues) => {
    const sanitized = sanitizeValues(values)
    const baseline = sanitizeValues(currentDefaults)
    const updates: Array<{ key: string; value: string | boolean | number }> = []

    for (const key of storageKeys) {
      if (sanitized[key] !== baseline[key]) {
        updates.push({
          key,
          value: sanitized[key],
        })
      }
    }

    if (updates.length === 0) {
      toast.message(t('No storage settings were changed'))
      return
    }

    await updateOptions.mutateAsync({ options: updates })
  }

  const resetToRecommendedDefaults = () => {
    form.reset(storageRecommendedDefaults)
  }

  return (
    <SettingsSection title={t('Storage Settings')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)} autoComplete='off'>

          <FormField
            control={form.control}
            name='ArchiveEnabled'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>
                    <FieldLabel meta={fieldMeta.ArchiveEnabled} t={t} />
                  </FormLabel>
                </SettingsSwitchContent>
                <FormControl>
                  <Switch
                    checked={field.value}
                    onCheckedChange={checked => {
                      field.onChange(checked)
                      if (checked) {
                        form.reset(withRecommendedDefaults(form.getValues()))
                      }
                    }}
                  />
                </FormControl>
              </SettingsSwitchItem>
            )}
          />

          {!archiveEnabled ? (
            <SettingsControlGroup>
              <GroupHeader
                title={t('Archive configuration is hidden until storage is enabled')}
                description={t(
                  'Turn on archive storage first. This keeps the operations page concise and avoids editing inactive settings by mistake.'
                )}
              />
            </SettingsControlGroup>
          ) : null}

          {archiveEnabled ? (
            <>
              <div
                data-settings-form-span='full'
                className='space-y-5 rounded-xl border px-4 py-4'
              >
                <GroupHeader
                  title={t('Capture strategy')}
                  description={t(
                    'Choose the final backend and the archive sampling ratio before tuning lower-level throughput controls.'
                  )}
                />
                <SettingsFormGrid className='pt-2'>
                  <FormField
                    control={form.control}
                    name='ArchiveBackend'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>
                            <FieldLabel meta={fieldMeta.ArchiveBackend} t={t} />
                        </FormLabel>
                        <FormControl>
                          <Select
                            value={field.value}
                            onValueChange={(value) => field.onChange(value)}
                          >
                            <SelectTrigger>
                              <SelectValue />
                            </SelectTrigger>
                            <SelectContent>
                              <SelectItem value='local'>{t('Local')}</SelectItem>
                              <SelectItem value='azure_blob'>
                                {t('AzureBlob')}
                              </SelectItem>
                            </SelectContent>
                          </Select>
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />

                  <FormField
                    control={form.control}
                    name='ArchiveSamplePercent'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>
                            <FieldLabel meta={fieldMeta.ArchiveSamplePercent} t={t} />
                        </FormLabel>
                        <FormControl>
                          <Input
                            type='number'
                            min={0}
                            max={100}
                            placeholder={formatPlaceholder(
                              t,
                              'ArchiveSamplePercent'
                            )}
                            {...safeNumberFieldProps(field)}
                          />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                </SettingsFormGrid>
              </div>

              <div
                data-settings-form-span='full'
                className='border-border/60 space-y-5 border-t pt-5'
              >
                <GroupHeader
                  title={t('Local staging paths')}
                  description={t(
                    'These directories are used by both local and Azure backends because request and response bodies are staged locally before finalization.'
                  )}
                />
                <SettingsFormGrid className='pt-2'>
                  <FormField
                    control={form.control}
                    name='ArchiveLocalDir'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>
                            <FieldLabel meta={fieldMeta.ArchiveLocalDir} t={t} />
                        </FormLabel>
                        <FormControl>
                          <Input
                            {...field}
                            placeholder={formatPlaceholder(
                              t,
                              'ArchiveLocalDir'
                            )}
                          />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />

                  <FormField
                    control={form.control}
                    name='ArchiveSpoolDir'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>
                            <FieldLabel meta={fieldMeta.ArchiveSpoolDir} t={t} />
                        </FormLabel>
                        <FormControl>
                          <Input
                            {...field}
                            placeholder={formatPlaceholder(
                              t,
                              'ArchiveSpoolDir'
                            )}
                          />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                </SettingsFormGrid>
              </div>

              <div
                data-settings-form-span='full'
                className='border-border/60 space-y-5 border-t pt-5'
              >
                <GroupHeader
                  title={t('Throughput and object limits')}
                  description={t(
                    'These values control queue depth, background concurrency, and the maximum body size that is eligible for capture.'
                  )}
                />
                <SettingsFormGrid className='pt-2'>
                  {(
                    [
                      'ArchiveQueueSize',
                      'ArchiveWorkerCount',
                      'ArchiveMaxRequestMB',
                      'ArchiveMaxResponseMB',
                      'ArchiveSpoolTTLHours',
                      'ArchiveHeaderValueMaxLength',
                    ] as const
                  ).map(key => (
                    <FormField
                      key={key}
                      control={form.control}
                      name={key}
                      render={({ field }) => (
                        <FormItem>
                          <FormLabel>
                            <FieldLabel meta={fieldMeta[key]} t={t} />
                          </FormLabel>
                          <FormControl>
                            <Input
                              type='number'
                              min={0}
                              placeholder={formatPlaceholder(t, key)}
                              {...safeNumberFieldProps(field)}
                            />
                          </FormControl>
                          <FormMessage />
                        </FormItem>
                      )}
                    />
                  ))}
                </SettingsFormGrid>
              </div>

              <div
                data-settings-form-span='full'
                className='border-border/60 space-y-5 border-t pt-5'
              >
                <GroupHeader
                  title={t('Segment aggregation')}
                  description={t(
                    'Small payloads can be appended into segment files. Large payloads stay on per-request finalization to avoid oversized hot files.'
                  )}
                />
                <SettingsFormGrid className='pt-2'>
                  {(
                    [
                      'ArchiveSmallPayloadMaxKB',
                      'ArchiveSegmentMaxMB',
                      'ArchiveSegmentMaxAgeSeconds',
                      'ArchiveSegmentMaxRecords',
                      'ArchiveSegmentShardCount',
                    ] as const
                  ).map(key => (
                    <FormField
                      key={key}
                      control={form.control}
                      name={key}
                      render={({ field }) => (
                        <FormItem>
                          <FormLabel>
                            <FieldLabel meta={fieldMeta[key]} t={t} />
                          </FormLabel>
                          <FormControl>
                            <Input
                              type='number'
                              min={0}
                              placeholder={formatPlaceholder(t, key)}
                              {...safeNumberFieldProps(field)}
                            />
                          </FormControl>
                          <FormMessage />
                        </FormItem>
                      )}
                    />
                  ))}
                </SettingsFormGrid>
              </div>

              <div
                data-settings-form-span='full'
                className='border-border/60 space-y-5 border-t pt-5'
              >
                <GroupHeader
                  title={t('Load shedding protection')}
                  description={t(
                    'These gates decide when archive work should be skipped to protect the main relay path under CPU, memory, or disk pressure.'
                  )}
                />
                <SettingsFormGrid className='pt-2'>
                  <SettingsFormGridItem span='full'>
                    <FormField
                      control={form.control}
                      name='ArchiveSkipOnHighLoad'
                      render={({ field }) => (
                        <SettingsSwitchItem className='py-0'>
                          <SettingsSwitchContent>
                            <FormLabel>
                              <FieldLabel meta={fieldMeta.ArchiveSkipOnHighLoad} t={t} />
                            </FormLabel>
                          </SettingsSwitchContent>
                          <FormControl>
                            <Switch
                              checked={field.value}
                              onCheckedChange={field.onChange}
                            />
                          </FormControl>
                        </SettingsSwitchItem>
                      )}
                    />
                  </SettingsFormGridItem>

                  {(
                    [
                      'ArchiveMaxCPUPercent',
                      'ArchiveMaxMemoryPercent',
                      'ArchiveMinFreeDiskPercent',
                      'ArchiveMinFreeDiskGB',
                      'ArchiveLoadCheckIntervalSeconds',
                    ] as const
                  ).map(key => (
                    <FormField
                      key={key}
                      control={form.control}
                      name={key}
                      render={({ field }) => (
                        <FormItem>
                          <FormLabel>
                            <FieldLabel meta={fieldMeta[key]} t={t} />
                          </FormLabel>
                          <FormControl>
                            <Input
                              type='number'
                              min={0}
                              placeholder={formatPlaceholder(t, key)}
                              {...safeNumberFieldProps(field)}
                            />
                          </FormControl>
                          <FormMessage />
                        </FormItem>
                      )}
                    />
                  ))}
                </SettingsFormGrid>
              </div>

              {backend === 'azure_blob' ? (
                <div
                  data-settings-form-span='full'
                  className='border-border/60 space-y-5 border-t pt-5'
                >
                  <GroupHeader
                    title={t('Azure Blob connection')}
                    description={t(
                      'These credentials are only needed when finalized archive objects are uploaded into Azure Blob Storage.'
                    )}
                  />
                  <SettingsFormGrid className='pt-2'>
                    {(
                      [
                        'ArchiveAzureAccountURL',
                        'ArchiveAzureContainer',
                        'ArchiveAzureAccountName',
                        'ArchiveAzureAccountKey',
                      ] as const
                    ).map(key => (
                      <FormField
                        key={key}
                        control={form.control}
                        name={key}
                        render={({ field }) => (
                          <FormItem>
                            <FormLabel>
                              <FieldLabel meta={fieldMeta[key]} t={t} />
                            </FormLabel>
                            <FormControl>
                              <Input
                                type={
                                  key === 'ArchiveAzureAccountKey'
                                    ? 'password'
                                    : 'text'
                                }
                                autoComplete='new-password'
                                placeholder={formatPlaceholder(t, key)}
                                {...field}
                              />
                            </FormControl>
                            <FormMessage />
                          </FormItem>
                        )}
                      />
                    ))}
                  </SettingsFormGrid>
                </div>
              ) : null}
            </>
          ) : null}
          <div
            data-settings-form-span='full'
            className='flex flex-wrap items-center justify-start gap-2 pt-2'
          >
            <Button
              type='button'
              variant='outline'
              onClick={resetToRecommendedDefaults}
              disabled={updateOptions.isPending}
            >
              {t('Reset to defaults')}
            </Button>
            <Button
              type='button'
              onClick={form.handleSubmit(onSubmit)}
              disabled={updateOptions.isPending}
            >
              {updateOptions.isPending
                ? t('Saving...')
                : t('Save storage settings')}
            </Button>
          </div>
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
