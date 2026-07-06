/** @type {import('next').NextConfig} */
const backend = process.env.BACKEND_ORIGIN ?? "http://localhost:8080";

const nextConfig = {
  // Proxy API calls to the Go backend so the session cookie stays same-site.
  async rewrites() {
    return [{ source: "/api/:path*", destination: `${backend}/api/:path*` }];
  },
  images: {
    remotePatterns: [{ protocol: "https", hostname: "cards.scryfall.io" }],
  },
};

export default nextConfig;
