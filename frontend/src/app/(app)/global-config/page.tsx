'use client'

import { PageHeader } from '@/components/layout/page-header'
import { SectionCard } from '@/components/shared/section-card'
import { Skeleton } from '@/components/ui/skeleton'
import { Settings } from 'lucide-react'
import { useAuth } from '@/hooks/use-auth'
import { RequireAdminRole } from '@/components/auth/require-admin-role'
import { useGlobalConfig, useGlobalConfigChanges } from '@/features/global-config/api/use-global-config'
import { GlobalConfigStructured } from '@/features/global-config/components/global-config-structured'
import { GlobalConfigChangeLog } from '@/features/global-config/components/global-config-change-log'
import { GlobalConfigJsonView } from '@/features/global-config/components/global-config-json-view'

function GlobalConfigContent() {
  const { user } = useAuth()
  const canAccessGlobalConfig = user?.role === 'admin'
  const { data: globalConfig, isLoading: configLoading, error: configError } = useGlobalConfig(canAccessGlobalConfig)
  const { data: changes, isLoading: changesLoading, error: changesError } = useGlobalConfigChanges(50, 0, false)
  
  const is404Error = changesError?.message?.includes('404')

  if (!canAccessGlobalConfig) {
    return (
      <div>
        <PageHeader
          title="Global Config"
          description="Platform-wide runtime configuration"
        />
        <SectionCard title="Access Denied" className="border-t-4 border-t-rose-500">
          <div className="text-center py-8 text-muted-foreground">
            <Settings className="h-12 w-12 mx-auto mb-4 opacity-50" />
            <p className="text-destructive mb-2">You do not have access to Global Config.</p>
            <p className="text-sm">Only admin users can view and modify platform-wide configuration.</p>
          </div>
        </SectionCard>
      </div>
    )
  }

  if (configLoading) {
    return (
      <div>
        <PageHeader
          title="Global Config"
          description="Platform-wide configuration and defaults"
        />
        <SectionCard title="Loading..." className="border-t-4 border-t-rose-500">
          <div className="space-y-4">
            <Skeleton className="h-8 w-full" />
            <Skeleton className="h-8 w-full" />
            <Skeleton className="h-8 w-full" />
          </div>
        </SectionCard>
      </div>
    )
  }

  if (configError) {
    return (
      <div>
        <PageHeader
          title="Global Config"
          description="Platform-wide configuration and defaults"
        />
        <SectionCard title="Error" className="border-t-4 border-t-rose-500">
          <div className="text-center py-8">
            <p className="text-destructive mb-2">Failed to load global configuration</p>
            <p className="text-sm text-muted-foreground">{configError.message}</p>
          </div>
        </SectionCard>
      </div>
    )
  }

  if (!globalConfig) {
    return (
      <div>
        <PageHeader
          title="Global Config"
          description="Platform-wide configuration and defaults"
        />
        <SectionCard title="No Configuration" className="border-t-4 border-t-rose-500">
          <div className="text-center py-8 text-muted-foreground">
            <Settings className="h-12 w-12 mx-auto mb-4 opacity-50" />
            <p>No global configuration available</p>
          </div>
        </SectionCard>
      </div>
    )
  }

  return (
    <div>
      <PageHeader
        title="Global Config"
        description="Platform-wide runtime configuration"
      />

      <div className="space-y-6">
        {/* Structured Config Sections */}
        <GlobalConfigStructured config={globalConfig.config} version={globalConfig.version} />

        {/* Raw JSON View */}
        <SectionCard title="Advanced" className="border-t-4 border-t-slate-500">
          <GlobalConfigJsonView config={globalConfig.config} />
        </SectionCard>

        {/* Change Log - Only show if endpoint is available */}
        {!is404Error && (
          <SectionCard
            title="Recent Changes"
            description="Global configuration change history"
            className="border-t-4 border-t-indigo-500"
          >
            <GlobalConfigChangeLog
              changes={changes?.data}
              isLoading={changesLoading}
              error={changesError}
            />
          </SectionCard>
        )}
      </div>
    </div>
  )
}

export default function GlobalConfigPage() {
  return (
    <RequireAdminRole>
      <GlobalConfigContent />
    </RequireAdminRole>
  )
}
