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
import { zodResolver } from '@hookform/resolvers/zod'
import type { Resolver } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { z } from 'zod'

import { PasswordInput } from '@/components/password-input'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'

import { FormDirtyIndicator } from '../components/form-dirty-indicator'
import { FormNavigationGuard } from '../components/form-navigation-guard'
import { SettingsForm } from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useSettingsForm } from '../hooks/use-settings-form'
import { useUpdateOption } from '../hooks/use-update-option'
import { safeNumberFieldProps } from '../utils/numeric-field'

const createExternalBillingSchema = (
  t: (key: string) => string,
  accessTokenConfigured: boolean
) =>
  z.object({
    BillSupplierName: z
      .string()
      .trim()
      .min(1, t('Supplier name is required'))
      .max(128),
    BillSiteURL: z
      .string()
      .trim()
      .min(1, t('Billing site URL is required'))
      .url(t('Must be a valid URL'))
      .refine(
        (value) => value.startsWith('http://') || value.startsWith('https://'),
        t('Must be a valid HTTP or HTTPS URL')
      ),
    BillAccessToken: z
      .string()
      .trim()
      .max(512)
      .refine(
        (value) => accessTokenConfigured || value.length > 0,
        t('Billing access token is required')
      ),
    BillDiscount: z.coerce
      .number()
      .gt(0, t('Discount must be greater than 0 and at most 1'))
      .max(1, t('Discount must be greater than 0 and at most 1')),
    BillPricingCurrency: z
      .string()
      .trim()
      .length(3, t('Currency code must contain 3 letters'))
      .regex(/^[a-zA-Z]{3}$/, t('Currency code must contain 3 letters'))
      .transform((value) => value.toUpperCase()),
  })

type ExternalBillingFormValues = z.infer<
  ReturnType<typeof createExternalBillingSchema>
>

type ExternalBillingSettingsSectionProps = {
  defaultValues: ExternalBillingFormValues
  accessTokenConfigured: boolean
}

export function ExternalBillingSettingsSection(
  props: ExternalBillingSettingsSectionProps
) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const schema = createExternalBillingSchema(t, props.accessTokenConfigured)

  const { form, handleSubmit, handleReset, isDirty, isSubmitting } =
    useSettingsForm<ExternalBillingFormValues>({
      resolver: zodResolver(schema) as Resolver<
        ExternalBillingFormValues,
        unknown,
        ExternalBillingFormValues
      >,
      defaultValues: props.defaultValues,
      onSubmit: async (_data, changedFields) => {
        for (const [key, value] of Object.entries(changedFields)) {
          if (key === 'BillAccessToken' && value === '') continue
          await updateOption.mutateAsync({
            key,
            value: value as string | number,
          })
        }
      },
    })

  return (
    <SettingsSection title={t('External Billing')}>
      <FormNavigationGuard when={isDirty} />

      <Form {...form}>
        <SettingsForm onSubmit={handleSubmit} autoComplete='off'>
          <SettingsPageFormActions
            onSave={handleSubmit}
            onReset={handleReset}
            isSaving={updateOption.isPending || isSubmitting}
            isSaveDisabled={!isDirty}
            isResetDisabled={!isDirty}
            saveLabel='Save external billing settings'
          />
          <FormDirtyIndicator isDirty={isDirty} />

          <FormField
            control={form.control}
            name='BillAccessToken'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Billing Access Token')}</FormLabel>
                <FormControl>
                  <PasswordInput
                    {...field}
                    maxLength={512}
                    autoComplete='new-password'
                  />
                </FormControl>
                <FormDescription>
                  {props.accessTokenConfigured
                    ? t(
                        'A billing access token is already configured. Leave blank to keep it, or enter a new token to rotate it.'
                      )
                    : t(
                        'Token required in the X-Access-Token header when calling external billing APIs'
                      )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='BillSupplierName'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Supplier Name')}</FormLabel>
                <FormControl>
                  <Input
                    {...field}
                    maxLength={128}
                    placeholder={t('Supplier Name')}
                  />
                </FormControl>
                <FormDescription>
                  {t('Name returned in supplierName')}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='BillSiteURL'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Billing Site URL')}</FormLabel>
                <FormControl>
                  <Input
                    {...field}
                    type='url'
                    maxLength={255}
                    placeholder='https://api.example.com'
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'URL returned in siteUrl. This setting is independent from the public site address.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='BillDiscount'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Billing Discount')}</FormLabel>
                <FormControl>
                  <Input
                    type='number'
                    min='0.000001'
                    max='1'
                    step='0.01'
                    {...safeNumberFieldProps(field)}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'Settlement multiplier used to calculate settleCost. Use 1 for no discount.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='BillPricingCurrency'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Billing Currency')}</FormLabel>
                <FormControl>
                  <Input
                    {...field}
                    maxLength={3}
                    placeholder='USD'
                    className='uppercase'
                  />
                </FormControl>
                <FormDescription>
                  {t('Three-letter currency code returned in pricingCurrency')}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
