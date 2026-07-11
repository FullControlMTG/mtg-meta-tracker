"use client";

import Link from "next/link";
import { apiPost } from "@/lib/api";
import { useSession } from "@/components/SessionProvider";

// Analytics is the merged stats page and lives per-cube; the bare path redirects
// to the default cube, so the nav does not need to know which cube that is.
const links = [
  { href: "/analytics", label: "Analytics" },
  { href: "/cubes", label: "Cubes" },
  { href: "/decks", label: "Decks" },
];

export function Nav() {
  const { me, refresh } = useSession();

  async function signOut() {
    try {
      await apiPost("/auth/logout");
    } catch {
      // ignore — refresh clears the client view regardless
    }
    // Re-reads the (now cleared) session, revalidates server pages, and
    // broadcasts the logout to the user's other tabs — no hard reload.
    await refresh();
  }

  return (
    <nav
      style={{
        borderBottom: "1px solid var(--border)",
        background: "var(--surface)",
      }}
    >
      <div
        style={{
          maxWidth: 1040,
          margin: "0 auto",
          padding: "0.75rem 1.5rem",
          display: "flex",
          alignItems: "center",
          gap: "1.25rem",
        }}
      >
        <Link href="/" style={{ fontWeight: 700, color: "var(--text)" }}>
          🎴 Meta Tracker
        </Link>
        <div style={{ display: "flex", gap: "1rem", flex: 1 }}>
          {links.map((l) => (
            <Link key={l.href} href={l.href} style={{ color: "var(--text-secondary)" }}>
              {l.label}
            </Link>
          ))}
          {me?.role === "admin" && (
            <Link
              href="/admin/cubes"
              style={{
                color: "var(--accent)",
                padding: "2px 8px",
                borderRadius: 999,
                border: "1px solid var(--border)",
                fontSize: "0.9rem",
              }}
            >
              Admin
            </Link>
          )}
        </div>
        {me === undefined ? (
          <span style={{ color: "var(--text-secondary)", opacity: 0.5 }}>…</span>
        ) : me === null ? (
          <Link href="/login" style={{ color: "var(--text-secondary)" }}>
            Sign in
          </Link>
        ) : (
          <div style={{ display: "flex", alignItems: "center", gap: "1rem" }}>
            <Link href={`/users/${me.username}`} style={{ color: "var(--text)" }}>
              {me.display_name || me.username}
            </Link>
            <Link href="/settings" style={{ color: "var(--text-secondary)" }}>
              Settings
            </Link>
            <button
              onClick={signOut}
              style={{
                background: "none",
                border: "none",
                padding: 0,
                cursor: "pointer",
                color: "var(--text-secondary)",
                font: "inherit",
              }}
            >
              Sign out
            </button>
          </div>
        )}
      </div>
    </nav>
  );
}
