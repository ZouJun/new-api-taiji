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
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { FormDescription, FormLabel } from '@/components/ui/form'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'

type FrontendThemeSectionProps = {
  frontendTheme: string
}

export function FrontendThemeSection({
  frontendTheme,
}: FrontendThemeSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const normalizedTheme = frontendTheme === 'classic' ? 'classic' : 'default'
  const [value, setValue] = useState<'default' | 'classic'>(
    normalizedTheme as 'default' | 'classic'
  )

  useEffect(() => {
    setValue(normalizedTheme as 'default' | 'classic')
  }, [normalizedTheme])

  const hasChanges = value !== normalizedTheme

  const onReset = () => {
    setValue(normalizedTheme as 'default' | 'classic')
  }

  const onSave = async () => {
    await updateOption.mutateAsync({
      key: 'theme.frontend',
      value,
    })
  }

  return (
    <SettingsSection title={t('Frontend Theme')}>
      <div className='space-y-4'>
        <div className='space-y-2'>
          <FormLabel>{t('Frontend Theme')}</FormLabel>
          <Select
            items={[
              {
                value: 'default',
                label: t('Default (New Frontend)'),
              },
              {
                value: 'classic',
                label: t('Classic (Legacy Frontend)'),
              },
            ]}
            onValueChange={(next) => setValue(next as 'default' | 'classic')}
            value={value}
          >
            <SelectTrigger className='w-full max-w-sm'>
              <SelectValue />
            </SelectTrigger>
            <SelectContent alignItemWithTrigger={false}>
              <SelectGroup>
                <SelectItem value='default'>
                  {t('Default (New Frontend)')}
                </SelectItem>
                <SelectItem value='classic'>
                  {t('Classic (Legacy Frontend)')}
                </SelectItem>
              </SelectGroup>
            </SelectContent>
          </Select>
          <FormDescription>
            {t(
              'Switch between the new frontend and the classic frontend. Changes take effect after page reload.'
            )}
          </FormDescription>
        </div>

        <SettingsPageFormActions
          onSave={onSave}
          onReset={onReset}
          isSaving={updateOption.isPending}
          isSaveDisabled={!hasChanges}
          isResetDisabled={!hasChanges}
        />
      </div>
    </SettingsSection>
  )
}
