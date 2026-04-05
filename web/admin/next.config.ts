import type { NextConfig } from "next";

const isDev = process.env.NODE_ENV === "development";

const nextConfig: NextConfig = {
  // Static export for production (embedded in Go binary)
  // Disabled in dev mode to enable rewrites proxy
  ...(isDev ? {} : { output: "export" as const }),
  basePath: "/admin",
  trailingSlash: true,
  webpack: (config) => {
    config.watchOptions = {
      ...config.watchOptions,
      ignored: /node_modules/,
    };
    return config;
  },
  // Dev mode: proxy API requests to Go backend
  async rewrites() {
    return isDev
      ? [
          {
            source: "/bot/api/:path*",
            destination: "http://localhost:18080/bot/api/:path*",
          },
        ]
      : [];
  },
};

export default nextConfig;
