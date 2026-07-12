"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import type { ReactNode } from "react";
import { useSession } from "@/components/SessionProvider";

const tabs = [
  { href: "/admin/cubes", label: "Cubes" },
  { href: "/admin/users", label: "Users" },
];

// Tab bar shared by the admin pages. Each page still gates its own content on
// the session — this only decides whether to draw the tabs at all.
export default function AdminLayout({ children }: { children: ReactNode }) {
  const { me } = useSession();
  const pathname = usePathname();

  return (
    <>
      {me?.role === "admin" && (
        <div
          style={{
            maxWidth: 1040,
            margin: "0 auto",
            padding: "1rem 1.5rem 0",
            display: "flex",
            gap: "1rem",
            // Same rule as the main nav: scroll sideways before wrapping onto a second line.
            whiteSpace: "nowrap",
            overflowX: "auto",
          }}
        >
          {tabs.map((t) => (
            <Link
              key={t.href}
              href={t.href}
              style={{
                color: pathname === t.href ? "var(--text)" : "var(--text-secondary)",
                fontWeight: pathname === t.href ? 600 : 400,
              }}
            >
              {t.label}
            </Link>
          ))}
        </div>
      )}
      {children}
    </>
  );
}
