import type { NextConfig } from 'next'
import './env' // Validate env vars at build time

const nextConfig: NextConfig = {
  experimental: {
    reactCompiler: false, // We'll enable this when React Compiler is stable
    serverActions: {
      bodySizeLimit: '2mb',
    },
  },
  logging: {
    fetches: {
      fullUrl: true,
    },
  },
  // Proxy API requests to the Go backend during development
  async rewrites() {
    return [
      {
        source: '/api/v1/:path*',
        destination: process.env.BACKEND_URL || 'http://localhost:8080/api/v1/:path*',
      },
    ]
  },
  images: {
    domains: [],
  },
  poweredByHeader: false,
  compress: true,
  reactStrictMode: true,
}

export default nextConfig
