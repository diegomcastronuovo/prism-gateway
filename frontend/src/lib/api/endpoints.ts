export const endpoints = {
  auth: {
    login: '/auth/login',
    logout: '/auth/logout',
    refresh: '/auth/refresh',
    me: '/auth/me',
  },
  tenants: {
    list: '/tenants',
    get: (id: string) => `/tenants/${id}`,
    create: '/tenants',
    update: (id: string) => `/tenants/${id}`,
    delete: (id: string) => `/tenants/${id}`,
  },
  routing: {
    list: '/routing',
    get: (id: string) => `/routing/${id}`,
    create: '/routing',
    update: (id: string) => `/routing/${id}`,
  },
  semantic: {
    list: '/semantic',
    get: (id: string) => `/semantic/${id}`,
  },
  tools: {
    list: '/tools',
    get: (id: string) => `/tools/${id}`,
  },
  benchmarks: {
    list: '/benchmarks',
    get: (id: string) => `/benchmarks/${id}`,
    results: (id: string) => `/benchmarks/${id}/results`,
  },
  budgets: {
    list: '/budgets',
    get: (id: string) => `/budgets/${id}`,
    usage: (id: string) => `/budgets/${id}/usage`,
  },
  observability: {
    metrics: '/observability/metrics',
    logs: '/observability/logs',
    traces: '/observability/traces',
  },
  replay: {
    list: '/replay',
    get: (id: string) => `/replay/${id}`,
    execute: (id: string) => `/replay/${id}/execute`,
  },
  experiments: {
    list: '/experiments',
    get: (id: string) => `/experiments/${id}`,
    results: (id: string) => `/experiments/${id}/results`,
  },
} as const
