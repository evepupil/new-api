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
import { Link } from '@tanstack/react-router'
import { KeyRound, RefreshCw, Search, ShieldCheck } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Main } from '@/components/layout'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { usePricingData } from '@/features/pricing/hooks/use-pricing-data'
import {
  formatFixedPrice,
  formatGroupPrice,
} from '@/features/pricing/lib/price'
import { cn } from '@/lib/utils'

import { formatProductTime } from './lib/product-time'
import type { ManyRouterProduct } from './types'
import { useManyRouterProducts } from './use-products'

type ProductFilter = 'all' | 'fixed_auto' | 'dedicated'

export function ManyRouterProductsPage() {
  const { t, i18n } = useTranslation()
  const [search, setSearch] = useState('')
  const [filter, setFilter] = useState<ProductFilter>('all')
  const products = useManyRouterProducts()
  const pricing = usePricingData()
  const visible = useMemo(() => {
    const query = search.trim().toLocaleLowerCase()
    return (products.data?.products ?? []).filter((product) => {
      if (filter !== 'all' && product.kind !== filter) return false
      if (!query) return true
      return `${product.model} ${product.display_name}`
        .toLocaleLowerCase()
        .includes(query)
    })
  }, [filter, products.data?.products, search])
  const loading = products.isPending || pricing.isLoading
  const empty = !loading && !products.isError && visible.length === 0
  const ready = !loading && !products.isError && visible.length > 0

  return (
    <Main className='gap-5 overflow-auto p-4 sm:p-6'>
      <div className='flex flex-wrap items-start justify-between gap-4'>
        <div className='min-w-0'>
          <h1 className='text-xl font-semibold'>{t('Models & Auto')}</h1>
          {products.data && (
            <p className='text-muted-foreground mt-1 text-sm'>
              {products.data.site_name} · {t('Updated')}{' '}
              {formatProductTime(
                products.data.generated_at,
                i18n.resolvedLanguage || i18n.language
              )}
            </p>
          )}
        </div>
        <Button
          type='button'
          variant='outline'
          disabled={products.isFetching || pricing.isLoading}
          onClick={() => {
            void Promise.all([products.refetch(), pricing.refetch()])
          }}
        >
          <RefreshCw className={cn(products.isFetching && 'animate-spin')} />
          {t('Refresh')}
        </Button>
      </div>

      <div className='flex flex-wrap items-center justify-between gap-3'>
        <Tabs
          value={filter}
          onValueChange={(value) => setFilter(value as ProductFilter)}
        >
          <TabsList>
            <TabsTrigger value='all'>{t('All products')}</TabsTrigger>
            <TabsTrigger value='fixed_auto'>{t('Fixed Auto')}</TabsTrigger>
            <TabsTrigger value='dedicated'>{t('Dedicated routes')}</TabsTrigger>
          </TabsList>
        </Tabs>
        <div className='relative w-full sm:w-72'>
          <Search className='text-muted-foreground pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2' />
          <Input
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            placeholder={t('Search models')}
            aria-label={t('Search models')}
            className='pl-9'
          />
        </div>
      </div>

      {products.isError && (
        <Alert variant='destructive'>
          <AlertTitle>{t('Product data unavailable')}</AlertTitle>
          <AlertDescription>
            {t('API calls continue to use the current routing configuration.')}
          </AlertDescription>
        </Alert>
      )}

      {loading && (
        <div className='space-y-2' role='status' aria-label={t('Loading')}>
          {Array.from({ length: 5 }, (_, index) => (
            <Skeleton key={index} className='h-14 w-full' />
          ))}
        </div>
      )}
      {empty && (
        <div className='border-border text-muted-foreground flex min-h-44 items-center justify-center rounded-lg border text-sm'>
          {t('No available products')}
        </div>
      )}
      {ready && (
        <div
          className='border-border overflow-x-auto rounded-lg border'
          role='region'
          aria-label={t('Models & Auto')}
          tabIndex={0}
        >
          <Table className='min-w-[1040px]'>
            <TableHeader>
              <TableRow>
                <TableHead>{t('Model')}</TableHead>
                <TableHead>{t('Product')}</TableHead>
                <TableHead>{t('Current price')}</TableHead>
                <TableHead className='text-right'>
                  {t('Available suppliers')}
                </TableHead>
                <TableHead>{t('SLA (24h)')}</TableHead>
                <TableHead>{t('First token P50 / P95')}</TableHead>
                <TableHead>{t('Quality')}</TableHead>
                <TableHead>{t('Status')}</TableHead>
                <TableHead>{t('Updated')}</TableHead>
                <TableHead className='bg-background sticky right-0 text-right'>
                  {t('Actions')}
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {visible.map((product) => (
                <TableRow key={`${product.model}:${product.group_key}`}>
                  <TableCell className='max-w-64 min-w-32 font-medium break-words whitespace-normal'>
                    {product.model}
                  </TableCell>
                  <TableCell>
                    <div className='flex items-center gap-2'>
                      {product.kind === 'fixed_auto' && (
                        <ShieldCheck className='text-primary size-4' />
                      )}
                      <span>{product.display_name}</span>
                    </div>
                  </TableCell>
                  <TableCell>{priceForProduct(product, pricing, t)}</TableCell>
                  <TableCell className='text-right'>
                    {product.available_suppliers}
                    {product.failover_ready && (
                      <Badge variant='outline' className='text-success ml-2'>
                        {t('Failover ready')}
                      </Badge>
                    )}
                  </TableCell>
                  <TableCell>
                    {product.sla_percent == null
                      ? t('Insufficient data')
                      : `${product.sla_percent.toFixed(2)}%`}
                  </TableCell>
                  <TableCell>
                    {product.ttft_p50_ms == null || product.ttft_p95_ms == null
                      ? t('Insufficient data')
                      : `${product.ttft_p50_ms} / ${product.ttft_p95_ms} ms`}
                  </TableCell>
                  <TableCell>
                    <div>{qualityLabel(product.quality_grade, t)}</div>
                    <div className='text-muted-foreground mt-0.5 text-xs'>
                      {confidenceLabel(product.confidence, t)}
                    </div>
                  </TableCell>
                  <TableCell>
                    <Badge
                      variant='outline'
                      className={statusClass(product.status)}
                    >
                      {statusLabel(product.status, t)}
                    </Badge>
                  </TableCell>
                  <TableCell>
                    {product.facts_through
                      ? formatProductTime(
                          product.facts_through,
                          i18n.resolvedLanguage || i18n.language
                        )
                      : t('No data')}
                  </TableCell>
                  <TableCell className='bg-background sticky right-0 text-right'>
                    <Button
                      size='sm'
                      variant='outline'
                      render={
                        <Link
                          to='/keys'
                          search={{ create: true, group: product.group_key }}
                        />
                      }
                    >
                      <KeyRound />
                      {t('Create API Key')}
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}
    </Main>
  )
}

function priceForProduct(
  product: ManyRouterProduct,
  pricing: ReturnType<typeof usePricingData>,
  t: (key: string) => string
) {
  if (!product.price_version_id) {
    return t('Price pending confirmation')
  }
  const model = pricing.models.find(
    (candidate) => candidate.model_name === product.model
  )
  if (!model || pricing.groupRatio[product.group_key] == null) {
    return t('Price unavailable')
  }
  if (model.quota_type === 1) {
    return formatFixedPrice(
      model,
      product.group_key,
      false,
      pricing.priceRate,
      pricing.usdExchangeRate,
      pricing.groupRatio
    )
  }
  const input = formatGroupPrice(
    model,
    product.group_key,
    'input',
    'M',
    false,
    pricing.priceRate,
    pricing.usdExchangeRate,
    pricing.groupRatio
  )
  const output = formatGroupPrice(
    model,
    product.group_key,
    'output',
    'M',
    false,
    pricing.priceRate,
    pricing.usdExchangeRate,
    pricing.groupRatio
  )
  return (
    <span className='whitespace-normal'>
      {t('Input')} {input} · {t('Output')} {output}
    </span>
  )
}

function qualityLabel(
  value: ManyRouterProduct['quality_grade'],
  t: (key: string) => string
) {
  const labels = {
    excellent: 'Excellent',
    good: 'Good',
    fair: 'Fair',
    poor: 'Poor',
    insufficient: 'Insufficient data',
  } as const
  return t(labels[value])
}

function confidenceLabel(
  value: ManyRouterProduct['confidence'],
  t: (key: string) => string
) {
  const labels = {
    high: 'High confidence',
    medium: 'Medium confidence',
    low: 'Low confidence',
    insufficient: 'Insufficient data',
  } as const
  return t(labels[value])
}

function statusLabel(
  value: ManyRouterProduct['status'],
  t: (key: string) => string
) {
  const labels = {
    available: 'Available',
    insufficient: 'Insufficient data',
    stale: 'Data stale',
    unavailable: 'Unavailable',
    closed: 'Closed',
  } as const
  return t(labels[value])
}

function statusClass(value: ManyRouterProduct['status']) {
  if (value === 'available') return 'border-success/30 text-success'
  if (value === 'unavailable' || value === 'closed') {
    return 'border-destructive/30 text-destructive'
  }
  return 'border-warning/30 text-warning'
}
