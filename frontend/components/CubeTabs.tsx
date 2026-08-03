"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { cubePath } from "@/lib/cube";

// The tab bar for a cube. It lives on the cube layout, not the global nav — tabs are a
// property of the cube you're in, so they appear only once you're inside one.
export function CubeTabs({ cubeId, showManage }: { cubeId: string; showManage: boolean }) {
  const pathname = usePathname();
  const home = cubePath(cubeId);

  const tabs = [
    { href: home, label: "Overview", exact: true },
    { href: cubePath(cubeId, "/cards"), label: "Cards" },
    { href: cubePath(cubeId, "/decks"), label: "Decks" },
    ...(showManage ? [{ href: cubePath(cubeId, "/manage"), label: "Manage" }] : []),
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
