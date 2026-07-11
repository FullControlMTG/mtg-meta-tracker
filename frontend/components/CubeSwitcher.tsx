"use client";

import { useRouter } from "next/navigation";

// Every cube's stats live at their own URL, so switching cubes is a navigation,
// not local state — the selection stays in the address bar and is shareable.
export function CubeSwitcher({
  cubes,
  current,
}: {
  cubes: { id: string; name: string }[];
  current: string;
}) {
  const router = useRouter();
  if (cubes.length < 2) return null;

  return (
    <select
      value={current}
      onChange={(e) => router.push(`/analytics/${e.target.value}`)}
      aria-label="Cube"
      style={{ maxWidth: 260, width: "auto" }}
    >
      {cubes.map((c) => (
        <option key={c.id} value={c.id}>
          {c.name}
        </option>
      ))}
    </select>
  );
}
