"use client";

import { useEffect, useRef, useState } from "react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import type { PublicUser } from "@/lib/api";
import { useSignOut } from "@/components/SignOutButton";

// The account control in the top-right: a single name button that opens a dropdown
// (Profile / Invites / Settings / Sign out). Replaces the old row of inline links.
export function UserMenu({ me }: { me: PublicUser }) {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  const pathname = usePathname();
  const signOut = useSignOut();

  // Close on a click outside or Escape; a menu that only the toggle can dismiss traps
  // the pointer.
  useEffect(() => {
    if (!open) return;
    const onDown = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    };
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && setOpen(false);
    document.addEventListener("mousedown", onDown);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("mousedown", onDown);
      document.removeEventListener("keydown", onKey);
    };
  }, [open]);

  // Navigating closes it — the same link that moved the page shouldn't leave the menu
  // hanging open over the new one.
  useEffect(() => setOpen(false), [pathname]);

  return (
    <div className="user-menu" ref={ref}>
      <button
        type="button"
        className="user-menu-button"
        aria-haspopup="menu"
        aria-expanded={open}
        onClick={() => setOpen((v) => !v)}
      >
        {me.display_name || me.username}
        <span aria-hidden className="user-menu-caret">
          ▾
        </span>
      </button>
      {open && (
        <div className="user-menu-panel" role="menu">
          <Link href={`/users/${me.username}`} role="menuitem" className="user-menu-item">
            Profile
          </Link>
          <Link href="/invites" role="menuitem" className="user-menu-item">
            Invites
          </Link>
          <Link href="/settings" role="menuitem" className="user-menu-item">
            Settings
          </Link>
          <button type="button" role="menuitem" className="user-menu-item" onClick={signOut}>
            Sign out
          </button>
        </div>
      )}
    </div>
  );
}
