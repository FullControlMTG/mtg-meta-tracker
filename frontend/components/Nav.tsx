"use client";

import Link from "next/link";
import { useSession } from "@/components/SessionProvider";
import { UserMenu } from "@/components/UserMenu";

// The global nav is chrome only — the brand, an Admin entry for site admins, and the
// account menu. Cube navigation lives on the cube layout's tab bar, not here.
export function Nav() {
  const { me } = useSession();

  return (
    <nav className="nav">
      <div className="nav-row">
        {/* A logo that navigates home. It's a link (prefetch + right-click "open in tab"),
            styled to never underline — the image is the affordance, not text. Swap the
            emoji fallback by dropping a file at frontend/public/logo.png. */}
        <Link href="/" className="nav-brand" aria-label="Meta Tracker — home">
          {/* Falls back to just the wordmark until a file exists at public/logo.png. */}
          <img
            className="nav-logo"
            src="/logo.png"
            alt=""
            onError={(e) => {
              e.currentTarget.style.display = "none";
            }}
          />
          <span>Meta Tracker</span>
        </Link>
        <div className="nav-links">
          {me?.role === "admin" && (
            <Link href="/admin" className="nav-admin">
              Admin
            </Link>
          )}
        </div>
        {me === undefined ? (
          <span className="nav-link" style={{ opacity: 0.5 }}>
            …
          </span>
        ) : me === null ? (
          <Link href="/login" className="nav-link">
            Sign in
          </Link>
        ) : (
          <UserMenu me={me} />
        )}
      </div>
    </nav>
  );
}
