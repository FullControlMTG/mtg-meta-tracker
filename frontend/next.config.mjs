/** @type {import('next').NextConfig} */
const backend = process.env.BACKEND_ORIGIN ?? "http://localhost:8080";

const nextConfig = {
  output: "standalone",
  // Proxy API calls to the Go backend so the session cookie stays same-site.
  async rewrites() {
    return [{ source: "/api/:path*", destination: `${backend}/api/:path*` }];
  },
  // Deck pages moved to /decks/<uuid>; keep old links working. Note this only
  // affects page routes — the backend API is still /api/decklists/*, and the
  // rewrite above matches first.
  async redirects() {
    return [
      { source: "/decklists", destination: "/decks", permanent: true },
      { source: "/decklists/:path*", destination: "/decks/:path*", permanent: true },
    ];
  },
  images: {
    remotePatterns: [{ protocol: "https", hostname: "cards.scryfall.io" }],
  },
};

export default nextConfig;
