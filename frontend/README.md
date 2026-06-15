# AI Gateway Frontend

Production-ready frontend for the AI Gateway Control Plane built with Next.js, TypeScript, and modern UI frameworks.

## Tech Stack

- **Next.js 14** - React framework with App Router
- **TypeScript** - Type-safe development
- **Tailwind CSS** - Utility-first styling
- **shadcn/ui** - High-quality UI components
- **Lucide React** - Beautiful icons
- **TanStack Query** - Data fetching and caching
- **React Hook Form** - Form management
- **Zod** - Schema validation
- **next-themes** - Dark/light mode support

## Features

### ✅ Implemented

- **Authentication Architecture**
  - Mock session for development
  - Prepared for Keycloak and AWS Cognito integration
  - Auth guards and protected routes
  - Session management

- **Theme System**
  - Dark/light mode toggle
  - Accent color switcher (Blue, Violet, Green)
  - Persistent theme preferences

- **Layout System**
  - App shell with sidebar and topbar
  - Responsive navigation
  - Active route highlighting
  - Collapsible sidebar support

- **Placeholder Pages**
  - Dashboard with metrics overview
  - Tenants management
  - Routing configuration
  - Semantic routing
  - Tool routing
  - Benchmarks
  - Budgets
  - Observability
  - Replay sessions
  - Experiments
  - Settings

- **Reusable Components**
  - StatCard - Metric display cards
  - SectionCard - Content sections
  - EmptyState - Placeholder states
  - StatusBadge - Status indicators
  - LoadingScreen - Loading states
  - PageHeader - Page titles and actions

### 🔄 Pending Integration

- **Authentication Providers**
  - Keycloak OIDC integration
  - AWS Cognito integration

- **API Integration**
  - Backend API connection
  - Real data fetching
  - Mutation handling

## Getting Started

### Prerequisites

- Node.js 18+ and npm

### Installation

```bash
# Install dependencies
npm install

# Run development server
npm run dev
```

Open [http://localhost:3000](http://localhost:3000) to view the application.

### Development Login

Use the "Continue with Mock Session" button on the login page to access the application during development.

## Project Structure

```
src/
├── app/                    # Next.js App Router
│   ├── (app)/             # Protected app routes
│   │   ├── dashboard/
│   │   ├── tenants/
│   │   ├── routing/
│   │   └── ...
│   ├── (auth)/            # Auth routes
│   │   └── login/
│   ├── layout.tsx         # Root layout with providers
│   └── globals.css        # Global styles
│
├── components/
│   ├── layout/            # Layout components
│   │   ├── app-shell.tsx
│   │   ├── sidebar.tsx
│   │   ├── topbar.tsx
│   │   └── ...
│   ├── shared/            # Shared components
│   │   ├── empty-state.tsx
│   │   ├── stat-card.tsx
│   │   └── ...
│   └── ui/                # shadcn/ui components
│       ├── button.tsx
│       ├── card.tsx
│       └── ...
│
├── lib/
│   ├── api/               # API client layer
│   │   ├── client.ts
│   │   ├── fetcher.ts
│   │   └── endpoints.ts
│   ├── auth/              # Authentication
│   │   ├── auth-context.tsx
│   │   ├── guards.tsx
│   │   └── session.ts
│   ├── themes/            # Theme configuration
│   │   └── theme-config.ts
│   └── utils/             # Utilities
│       ├── cn.ts
│       ├── format.ts
│       └── constants.ts
│
├── hooks/                 # Custom React hooks
│   ├── use-auth.ts
│   ├── use-theme-accent.ts
│   └── use-sidebar-state.ts
│
└── types/                 # TypeScript types
    ├── api.ts
    ├── auth.ts
    └── common.ts
```

## Authentication Integration

### Keycloak Setup

1. Configure environment variables in `.env.local`:
```env
NEXT_PUBLIC_KEYCLOAK_URL=https://your-keycloak-url
NEXT_PUBLIC_KEYCLOAK_REALM=your-realm
NEXT_PUBLIC_KEYCLOAK_CLIENT_ID=your-client-id
```

2. Implement OIDC flow in `lib/auth/auth-context.tsx`

### AWS Cognito Setup

1. Configure environment variables in `.env.local`:
```env
NEXT_PUBLIC_COGNITO_REGION=us-east-1
NEXT_PUBLIC_COGNITO_USER_POOL_ID=your-pool-id
NEXT_PUBLIC_COGNITO_CLIENT_ID=your-client-id
```

2. Implement Cognito authentication in `lib/auth/auth-context.tsx`

## API Integration

The API client is prepared in `lib/api/` with support for:
- Bearer token authentication
- API key authentication
- Centralized error handling
- Type-safe endpoints

Configure the API base URL in `.env.local`:
```env
NEXT_PUBLIC_API_URL=http://localhost:8000
```

## Theme Customization

### Accent Colors

Modify `lib/themes/theme-config.ts` to add or change accent colors:

```typescript
export const themeAccents: Record<ThemeAccent, { primary: string; ring: string }> = {
  blue: { primary: '221.2 83.2% 53.3%', ring: '221.2 83.2% 53.3%' },
  // Add more colors...
}
```

### Color Scheme

Edit `src/app/globals.css` to customize the color palette for light and dark modes.

## Build

```bash
# Production build
npm run build

# Start production server
npm start
```

## Code Quality

```bash
# Lint code
npm run lint
```

## Architecture Decisions

### Authentication Layer
- Abstracted to support multiple providers (Keycloak, Cognito)
- Mock session for development
- Guards protect authenticated routes
- Session stored in localStorage

### API Layer
- Centralized client with interceptors
- Type-safe endpoint definitions
- Prepared for token refresh
- Error handling abstraction

### Theme System
- CSS variables for easy customization
- Persistent preferences
- Accent color system
- Dark/light mode support

### Component Architecture
- Reusable, composable components
- Consistent prop interfaces
- shadcn/ui for base primitives
- Custom components for domain logic

## Next Steps

1. **Integrate Authentication**
   - Implement Keycloak OIDC flow
   - Implement AWS Cognito flow
   - Add token refresh logic

2. **Connect Backend API**
   - Implement data fetching with TanStack Query
   - Add mutations for CRUD operations
   - Handle real-time updates

3. **Implement Business Logic**
   - Add forms for creating/editing resources
   - Implement data tables with sorting/filtering
   - Add charts and visualizations
   - Build workflow features

4. **Testing**
   - Add unit tests
   - Add integration tests
   - Add E2E tests

## License

Proprietary
