"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { apiGetOptional, apiPost, type PublicUser } from "@/lib/api";

const links = [
  { href: "/", label: "Overview" },
  { href: "/analytics", label: "Analytics" },
  { href: "/cubes", label: "Cubes" },
  { href: "/decklists", label: "Decklists" },
];

export function Nav() {
  // undefined = still loading, null = logged out, object = logged in
  const [me, setMe] = useState<PublicUser | null | undefined>(undefined);

  useEffect(() => {
    apiGetOptional<PublicUser>("/auth/me").then(setMe);
  }, []);

  async function signOut() {
    try {
      await apiPost("/auth/logout");
    } catch {
      // ignore — clear the client view regardless
    }
    window.location.assign("/");
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
