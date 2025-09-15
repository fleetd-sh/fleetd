import { createEnv } from '@t3-oss/env-nextjs'
import { z } from 'zod'

export const env = createEnv({
  /**
   * Server-side environment variables
   * These are only available on the server
   */
  server: {
    // Backend API URL for server-side requests
    BACKEND_URL: z.string().url().default('http://localhost:8080'),

    // Authentication secrets (future cloud offering)
    AUTH_SECRET: z.string().min(32).optional(),
    JWT_SECRET: z.string().min(32).optional(),

    // Database URL (if using direct DB access)
    DATABASE_URL: z.string().url().optional(),

    // External auth providers (cloud offering)
    AUTH0_CLIENT_ID: z.string().optional(),
    AUTH0_CLIENT_SECRET: z.string().optional(),
    AUTH0_DOMAIN: z.string().optional(),

    // GitHub OAuth (for SSO)
    GITHUB_CLIENT_ID: z.string().optional(),
    GITHUB_CLIENT_SECRET: z.string().optional(),

    // Google OAuth (for SSO)
    GOOGLE_CLIENT_ID: z.string().optional(),
    GOOGLE_CLIENT_SECRET: z.string().optional(),

    // Telemetry and monitoring
    TELEMETRY_ENDPOINT: z.string().url().optional(),
    SENTRY_DSN: z.string().url().optional(),

    // Feature flags
    ENABLE_SSO: z.coerce.boolean().default(false),
    ENABLE_MULTI_TENANCY: z.coerce.boolean().default(false),
    ENABLE_BILLING: z.coerce.boolean().default(false),

    // Node environment
    NODE_ENV: z.enum(['development', 'production', 'test']).default('development'),
  },

  /**
   * Client-side environment variables
   * These are exposed to the browser (prefixed with NEXT_PUBLIC_)
   */
  client: {
    // Public API URL for client-side requests
    NEXT_PUBLIC_API_URL: z.string().url().default('http://localhost:8080'),

    // App configuration
    NEXT_PUBLIC_APP_NAME: z.string().default('FleetD'),
    NEXT_PUBLIC_APP_VERSION: z.string().default('0.1.0'),

    // Feature flags (client-visible)
    NEXT_PUBLIC_ENABLE_ANALYTICS: z.coerce.boolean().default(false),
    NEXT_PUBLIC_ENABLE_DARK_MODE: z.coerce.boolean().default(true),
    NEXT_PUBLIC_ENABLE_TELEMETRY_CHARTS: z.coerce.boolean().default(true),

    // External services
    NEXT_PUBLIC_POSTHOG_KEY: z.string().optional(),
    NEXT_PUBLIC_POSTHOG_HOST: z.string().url().optional(),
    NEXT_PUBLIC_CRISP_WEBSITE_ID: z.string().optional(),

    // SSO providers (visible to client for login options)
    NEXT_PUBLIC_SSO_PROVIDERS: z.string().optional(), // comma-separated list

    // Cloud vs OSS mode
    NEXT_PUBLIC_DEPLOYMENT_MODE: z.enum(['oss', 'cloud']).default('oss'),

    // Support links
    NEXT_PUBLIC_DOCS_URL: z.string().url().default('https://docs.fleetd.sh'),
    NEXT_PUBLIC_SUPPORT_URL: z.string().url().optional(),
    NEXT_PUBLIC_GITHUB_URL: z.string().url().default('https://github.com/fleetd'),
  },

  /**
   * Runtime environment variables
   * You need to destructure all environment variables here
   */
  runtimeEnv: {
    // Server
    BACKEND_URL: process.env.BACKEND_URL,
    AUTH_SECRET: process.env.AUTH_SECRET,
    JWT_SECRET: process.env.JWT_SECRET,
    DATABASE_URL: process.env.DATABASE_URL,
    AUTH0_CLIENT_ID: process.env.AUTH0_CLIENT_ID,
    AUTH0_CLIENT_SECRET: process.env.AUTH0_CLIENT_SECRET,
    AUTH0_DOMAIN: process.env.AUTH0_DOMAIN,
    GITHUB_CLIENT_ID: process.env.GITHUB_CLIENT_ID,
    GITHUB_CLIENT_SECRET: process.env.GITHUB_CLIENT_SECRET,
    GOOGLE_CLIENT_ID: process.env.GOOGLE_CLIENT_ID,
    GOOGLE_CLIENT_SECRET: process.env.GOOGLE_CLIENT_SECRET,
    TELEMETRY_ENDPOINT: process.env.TELEMETRY_ENDPOINT,
    SENTRY_DSN: process.env.SENTRY_DSN,
    ENABLE_SSO: process.env.ENABLE_SSO,
    ENABLE_MULTI_TENANCY: process.env.ENABLE_MULTI_TENANCY,
    ENABLE_BILLING: process.env.ENABLE_BILLING,
    NODE_ENV: process.env.NODE_ENV,

    // Client
    NEXT_PUBLIC_API_URL: process.env.NEXT_PUBLIC_API_URL,
    NEXT_PUBLIC_APP_NAME: process.env.NEXT_PUBLIC_APP_NAME,
    NEXT_PUBLIC_APP_VERSION: process.env.NEXT_PUBLIC_APP_VERSION,
    NEXT_PUBLIC_ENABLE_ANALYTICS: process.env.NEXT_PUBLIC_ENABLE_ANALYTICS,
    NEXT_PUBLIC_ENABLE_DARK_MODE: process.env.NEXT_PUBLIC_ENABLE_DARK_MODE,
    NEXT_PUBLIC_ENABLE_TELEMETRY_CHARTS: process.env.NEXT_PUBLIC_ENABLE_TELEMETRY_CHARTS,
    NEXT_PUBLIC_POSTHOG_KEY: process.env.NEXT_PUBLIC_POSTHOG_KEY,
    NEXT_PUBLIC_POSTHOG_HOST: process.env.NEXT_PUBLIC_POSTHOG_HOST,
    NEXT_PUBLIC_CRISP_WEBSITE_ID: process.env.NEXT_PUBLIC_CRISP_WEBSITE_ID,
    NEXT_PUBLIC_SSO_PROVIDERS: process.env.NEXT_PUBLIC_SSO_PROVIDERS,
    NEXT_PUBLIC_DEPLOYMENT_MODE: process.env.NEXT_PUBLIC_DEPLOYMENT_MODE,
    NEXT_PUBLIC_DOCS_URL: process.env.NEXT_PUBLIC_DOCS_URL,
    NEXT_PUBLIC_SUPPORT_URL: process.env.NEXT_PUBLIC_SUPPORT_URL,
    NEXT_PUBLIC_GITHUB_URL: process.env.NEXT_PUBLIC_GITHUB_URL,
  },

  /**
   * Skip validation in specific environments
   */
  skipValidation: !!process.env.SKIP_ENV_VALIDATION,

  /**
   * Tell the library when we're in a server context
   */
  emptyStringAsUndefined: true,
})
