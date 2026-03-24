import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  output: "export",
  basePath: "/admin",
  trailingSlash: true,
  webpack: (config) => {
    config.watchOptions = {
      ...config.watchOptions,
      ignored: /node_modules/,
    };
    return config;
  },
};

export default nextConfig;
