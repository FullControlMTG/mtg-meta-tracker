"use client";

import Link from "next/link";
import { useSession } from "@/components/SessionProvider";
import { SignOutButton } from "@/components/SignOutButton";

// The global nav is chrome only — brand + account. Cube navigation (Overview, Cards,
// Decks, Manage) is a property of the cube you're in and lives on the cube layout's tab
// bar, so it appears once you're inside a cube rather than at the top level.
//
// One row, at every width. Under the breakpoint (.nav-wide / .nav-narrow in globals.css)
// the brand drops to its emoji and the account cluster drops to a ⚙ — which is why the
// settings page it opens carries the profile link and the Sign out button of its own.
export function Nav() {
  const { me } = useSession();

  return (
    <nav className="nav">
      <div className="nav-row">
        <Link href="/" className="nav-brand">
          🎴<span className="nav-wide"> Meta Tracker</span>
        </Link>
        <div className="nav-links">
          {me?.role === "admin" && (
            <Link href="/admin/users" className="nav-admin">
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
          <div className="nav-account">
            <Link href={`/users/${me.username}`} className="nav-user">
              {me.display_name || me.username}
            </Link>
            <Link href="/settings" className="nav-link nav-wide">
              Settings
            </Link>
            <SignOutButton className="nav-link nav-linkbtn nav-wide" />
            {/* The mobile stand-in for the two above — the settings page is where sign out went. */}
            <Link href="/settings" className="nav-link nav-narrow" aria-label="Settings">
              ⚙
            </Link>
          </div>
        )}
      </div>
    </nav>
  );
}
