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
import { Plus, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Field,
  FieldContent,
  FieldDescription,
  FieldGroup,
  FieldLabel,
  FieldTitle,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
} from '@/components/ui/input-group'
import { cn } from '@/lib/utils'

import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

import {
  SettingsControlGroup,
  SettingsSwitchField,
} from '../components/settings-form-layout'
import { numericDraftRegex, type VideoPriceRow } from './model-pricing-core'

export function PriceInput(props: {
  value: string
  placeholder?: string
  disabled?: boolean
  onChange: (value: string) => void
}) {
  return (
    <InputGroup>
      <InputGroupAddon>$</InputGroupAddon>
      <InputGroupInput
        inputMode='decimal'
        value={props.value}
        placeholder={props.placeholder}
        disabled={props.disabled}
        onChange={(event) => props.onChange(event.target.value)}
      />
      <InputGroupAddon align='inline-end'>$/1M</InputGroupAddon>
    </InputGroup>
  )
}

export function PriceLane(props: {
  title: string
  description: string
  placeholder: string
  value: string
  enabled: boolean
  disabled?: boolean
  onEnabledChange: (checked: boolean) => void
  onChange: (value: string) => void
}) {
  const { t } = useTranslation()
  const effectiveDisabled = props.disabled || !props.enabled

  return (
    <SettingsControlGroup
      className={cn('space-y-3', effectiveDisabled && 'opacity-75')}
      data-disabled={effectiveDisabled || undefined}
    >
      <SettingsSwitchField
        checked={props.enabled}
        disabled={props.disabled}
        onCheckedChange={props.onEnabledChange}
        label={props.title}
        description={props.description}
        aria-label={props.title}
      />
      <PriceInput
        value={props.value}
        placeholder={props.placeholder}
        disabled={effectiveDisabled}
        onChange={props.onChange}
      />
      <p className='text-muted-foreground text-xs'>
        {props.enabled
          ? t('USD price per 1M tokens.')
          : t('Disabled lanes are omitted on save.')}
      </p>
    </SettingsControlGroup>
  )
}

export function VideoPricingEditor(props: {
  baseFps: string
  inputContentPrice: string
  inputVideoPricePerSecond: string
  rows: VideoPriceRow[]
  nextRowId: number
  onBaseFpsChange: (value: string) => void
  onInputContentPriceChange: (value: string) => void
  onInputVideoPricePerSecondChange: (value: string) => void
  onRowsChange: (rows: VideoPriceRow[]) => void
  onNextRowIdChange: (value: number) => void
}) {
  const { t } = useTranslation()

  const updateRow = (
    id: number,
    field: 'resolution' | 'price',
    value: string
  ) => {
    props.onRowsChange(
      props.rows.map((row) =>
        row.id === id ? { ...row, [field]: value } : row
      )
    )
  }

  const addRow = () => {
    props.onRowsChange([
      ...props.rows,
      { id: props.nextRowId, resolution: '', price: '' },
    ])
    props.onNextRowIdChange(props.nextRowId + 1)
  }

  return (
    <FieldGroup>
      <Field>
        <FieldLabel>{t('Base FPS')}</FieldLabel>
        <Input
          inputMode='decimal'
          value={props.baseFps}
          placeholder='24'
          onChange={(event) => {
            if (numericDraftRegex.test(event.target.value)) {
              props.onBaseFpsChange(event.target.value)
            }
          }}
        />
        <FieldDescription>
          {t('Requests with higher FPS are multiplied by fps / base fps.')}
        </FieldDescription>
      </Field>

      <div className='grid gap-4 sm:grid-cols-2'>
        <Field>
          <FieldLabel>{t('Input content price')}</FieldLabel>
          <InputGroup>
            <InputGroupAddon>$</InputGroupAddon>
            <InputGroupInput
              inputMode='decimal'
              value={props.inputContentPrice}
              placeholder='0'
              onChange={(event) => {
                if (numericDraftRegex.test(event.target.value)) {
                  props.onInputContentPriceChange(event.target.value)
                }
              }}
            />
            <InputGroupAddon align='inline-end'>
              {t('per request')}
            </InputGroupAddon>
          </InputGroup>
          <FieldDescription>
            {t('Flat USD charge when the request includes input content.')}
          </FieldDescription>
        </Field>

        <Field>
          <FieldLabel>{t('Input video price')}</FieldLabel>
          <InputGroup>
            <InputGroupAddon>$</InputGroupAddon>
            <InputGroupInput
              inputMode='decimal'
              value={props.inputVideoPricePerSecond}
              placeholder='0'
              onChange={(event) => {
                if (numericDraftRegex.test(event.target.value)) {
                  props.onInputVideoPricePerSecondChange(event.target.value)
                }
              }}
            />
            <InputGroupAddon align='inline-end'>
              {t('/ sec')}
            </InputGroupAddon>
          </InputGroup>
          <FieldDescription>
            {t('Requires input_video_seconds when an input video is used.')}
          </FieldDescription>
        </Field>
      </div>

      <Field>
        <div className='flex items-center justify-between gap-3'>
          <FieldContent>
            <FieldTitle>{t('Resolution prices')}</FieldTitle>
            <FieldDescription>
              {t('Configure the output video price in USD per second.')}
            </FieldDescription>
          </FieldContent>
          <Button
            type='button'
            variant='outline'
            size='sm'
            onClick={addRow}
          >
            <Plus data-icon='inline-start' />
            {t('Add')}
          </Button>
        </div>
        <div className='overflow-hidden rounded-md border'>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('Resolution')}</TableHead>
                <TableHead>{t('Price per second')}</TableHead>
                <TableHead className='w-16 text-right'>
                  {t('Actions')}
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {props.rows.map((row) => (
                <TableRow key={row.id}>
                  <TableCell>
                    <Input
                      value={row.resolution}
                      placeholder='720p'
                      onChange={(event) =>
                        updateRow(row.id, 'resolution', event.target.value)
                      }
                    />
                  </TableCell>
                  <TableCell>
                    <InputGroup>
                      <InputGroupAddon>$</InputGroupAddon>
                      <InputGroupInput
                        inputMode='decimal'
                        value={row.price}
                        placeholder='0.1'
                        onChange={(event) => {
                          if (numericDraftRegex.test(event.target.value)) {
                            updateRow(row.id, 'price', event.target.value)
                          }
                        }}
                      />
                      <InputGroupAddon align='inline-end'>
                        {t('/ sec')}
                      </InputGroupAddon>
                    </InputGroup>
                  </TableCell>
                  <TableCell className='text-right'>
                    <Button
                      type='button'
                      variant='ghost'
                      size='icon'
                      onClick={() =>
                        props.onRowsChange(
                          props.rows.filter((item) => item.id !== row.id)
                        )
                      }
                      aria-label={t('Delete')}
                    >
                      <Trash2 className='text-destructive h-4 w-4' />
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      </Field>
    </FieldGroup>
  )
}
