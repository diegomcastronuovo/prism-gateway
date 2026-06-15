interface ServerRuntimeSectionProps {
  config: Record<string, unknown>
}

export function ServerRuntimeSection({ config }: ServerRuntimeSectionProps) {
  const server = config.server as Record<string, unknown> | undefined
  const database = config.database as Record<string, unknown> | undefined

  return (
    <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
      {server?.host !== undefined && (
        <div className="flex flex-col gap-2 p-4 rounded-lg border bg-card">
          <span className="text-xs text-muted-foreground">Host</span>
          <span className="font-medium text-sm font-mono">{String(server.host)}</span>
        </div>
      )}

      {server?.port !== undefined && (
        <div className="flex flex-col gap-2 p-4 rounded-lg border bg-card">
          <span className="text-xs text-muted-foreground">Port</span>
          <span className="font-medium text-sm">{Number(server.port)}</span>
        </div>
      )}

      {server?.request_timeout_ms !== undefined && (
        <div className="flex flex-col gap-2 p-4 rounded-lg border bg-card">
          <span className="text-xs text-muted-foreground">Request Timeout</span>
          <span className="font-medium text-sm">{Number(server.request_timeout_ms)} ms</span>
        </div>
      )}

      {database?.max_open_conns !== undefined && (
        <div className="flex flex-col gap-2 p-4 rounded-lg border bg-card">
          <span className="text-xs text-muted-foreground">DB Max Open Connections</span>
          <span className="font-medium text-sm">{Number(database.max_open_conns)}</span>
        </div>
      )}

      {database?.max_idle_conns !== undefined && (
        <div className="flex flex-col gap-2 p-4 rounded-lg border bg-card">
          <span className="text-xs text-muted-foreground">DB Max Idle Connections</span>
          <span className="font-medium text-sm">{Number(database.max_idle_conns)}</span>
        </div>
      )}
    </div>
  )
}
