import type { NextConfig } from "next";
import "./env"; // Validate env vars at build time

const nextConfig: NextConfig = {
  output: "standalone", // Enable standalone output for Docker
  experimental: {
    reactCompiler: false, // We'll enable this when React Compiler is stable
    serverActions: {
      bodySizeLimit: "2mb",
    },
  },
  logging: {
    fetches: {
      fullUrl: true,
    },
  },
  webpack: (config) => {
    // Handle .js imports in TypeScript files for protobuf-generated code
    config.resolve.extensionAlias = {
      ".js": [".js", ".ts"],
      ".jsx": [".jsx", ".tsx"],
    };

    // Also try fallback extensions
    config.resolve.extensions = [".tsx", ".ts", ".jsx", ".js", ".json"];

    return config;
  },
  // Proxy API requests to the Go backend during development
  async rewrites() {
    return [
      {
        source: "/api/v1/:path*",
        destination:
          process.env.DEVICE_API_URL ||
          process.env.BACKEND_URL ||
          "http://localhost:8080/api/v1/:path*",
      },
    ];
  },
  images: {
    domains: [],
  },
  poweredByHeader: false,
  compress: true,
  reactStrictMode: true,
};

export default nextConfig;
