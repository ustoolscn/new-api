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
import { BarChart3 } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'

import { RegisteredDevicesTable } from './components/registered-devices-table'
import { RegistrationStatistics } from './components/registration-statistics'
import { UsersDeleteDialog } from './components/users-delete-dialog'
import { UsersMutateDrawer } from './components/users-mutate-drawer'
import { UsersPrimaryButtons } from './components/users-primary-buttons'
import { UsersProvider, useUsers } from './components/users-provider'
import { UsersTable } from './components/users-table'

function UsersContent() {
  const { t } = useTranslation()
  const { open, setOpen, currentRow } = useUsers()
  const [activeTab, setActiveTab] = useState<
    'users' | 'devices' | 'statistics'
  >('users')

  return (
    <>
      <SectionPageLayout fixedContent>
        <SectionPageLayout.Title>{t('Users')}</SectionPageLayout.Title>
        <SectionPageLayout.Actions>
          {activeTab === 'users' && <UsersPrimaryButtons />}
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          <Tabs
            value={activeTab}
            onValueChange={(value) =>
              setActiveTab(value as 'users' | 'devices' | 'statistics')
            }
            className='h-full min-h-0'
          >
            <TabsList>
              <TabsTrigger value='users'>{t('Users')}</TabsTrigger>
              <TabsTrigger value='devices'>
                {t('Registered devices')}
              </TabsTrigger>
              <TabsTrigger value='statistics'>
                <BarChart3 data-icon='inline-start' />
                {t('Statistics')}
              </TabsTrigger>
            </TabsList>
            <TabsContent value='users' className='min-h-0 flex-1'>
              <UsersTable />
            </TabsContent>
            <TabsContent value='devices' className='min-h-0 flex-1'>
              <RegisteredDevicesTable />
            </TabsContent>
            <TabsContent
              value='statistics'
              className='min-h-0 flex-1 overflow-auto'
            >
              <RegistrationStatistics />
            </TabsContent>
          </Tabs>
        </SectionPageLayout.Content>
      </SectionPageLayout>

      <UsersMutateDrawer
        open={open === 'create' || open === 'update'}
        onOpenChange={(isOpen) => !isOpen && setOpen(null)}
        currentRow={open === 'update' ? currentRow || undefined : undefined}
      />
      <UsersDeleteDialog />
    </>
  )
}

export function Users() {
  return (
    <UsersProvider>
      <UsersContent />
    </UsersProvider>
  )
}
