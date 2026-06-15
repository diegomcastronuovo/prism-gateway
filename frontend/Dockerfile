# Frontend Dockerfile - Production-safe multi-stage build
# No source code included in runtime image
# Standalone output reduces size ~90% vs default build

# ============================================================================
# BUILDER STAGE - Compile Next.js app
# ============================================================================
FROM node:20-alpine AS builder

WORKDIR /build

# Copy package files (lighter than copying everything)
COPY package.json package-lock.json ./

# Install ALL dependencies (including devDependencies needed for build)
RUN npm ci

# Copy application source only for build
COPY . .

# Build-time env vars (NEXT_PUBLIC_* are baked into the bundle at compile time)
ARG NEXT_PUBLIC_WHITE_LABEL=false
ARG NEXT_PUBLIC_BRAND_NAME=
ARG NEXT_PUBLIC_BRAND_SUBTITLE=
ENV NEXT_PUBLIC_WHITE_LABEL=$NEXT_PUBLIC_WHITE_LABEL
ENV NEXT_PUBLIC_BRAND_NAME=$NEXT_PUBLIC_BRAND_NAME
ENV NEXT_PUBLIC_BRAND_SUBTITLE=$NEXT_PUBLIC_BRAND_SUBTITLE

# Build Next.js app with standalone output
# Standalone includes minimal Node.js runtime + app code, no source
RUN npm run build

# ============================================================================
# RUNTIME STAGE - Minimal production image
# ============================================================================
FROM node:20-alpine

WORKDIR /app

# Copy ONLY compiled standalone output from builder
# .next/standalone contains the minimal runtime app
COPY --from=builder /build/.next/standalone ./

# Copy static assets (.next/static is required even with standalone output)
COPY --from=builder /build/.next/static ./.next/static

# Copy public assets if they exist
COPY --from=builder /build/public ./public

# Create non-root user for security
RUN adduser -D -u 10001 nextjs
USER nextjs

# Expose Next.js default port
EXPOSE 3000

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --quiet --tries=1 --spider http://127.0.0.1:3000/api/health || exit 1

# Start Next.js production server
# Next.js standalone handles signals properly without dumb-init
# Set HOSTNAME to listen on all interfaces
ENV HOSTNAME 0.0.0.0
CMD ["node", "server.js"]
