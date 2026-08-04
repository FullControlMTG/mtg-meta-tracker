"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { cubePath } from "@/lib/cube";

// The tab bar for a cube. It lives on the cube layout, not the global nav — tabs are a
// property of the cube you're in, so they appear only once you're inside one. Managing a
// cube is a separate "Manage cube" button in the header, not a tab.
export function CubeTabs({ cubeId }: { cubeId: string }) {
  const pathname = usePathname();
  const home = cubePath(cubeId);

  const tabs = [
    { href: home, label: "Overview", exact: true },
    { href: cubePath(cubeId, "/cards"), label: "Cards" },
    { href: cubePath(cubeId, "/decks"), label: "Decks" },
    { href: cubePath(cubeId, "/combos"), label: "Combos" },
  ];

  return (
    <div className="cube-tabs">
      {tabs.map((t) => {
        const active = t.exact ? pathname === t.href : pathname.startsWith(t.href);
        return (
          <Link key={t.href} href={t.href} className={`cube-tab${active ? " active" : ""}`}>
            {t.label}
          </Link>
        );
      })}
    </div>
  );
}
