import Link from "next/link";

const links = [
  { href: "/", label: "Overview" },
  { href: "/analytics", label: "Analytics" },
  { href: "/decklists", label: "Decklists" },
  { href: "/decks/new", label: "New deck" },
];

export function Nav() {
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
        </div>
        <Link href="/login" style={{ color: "var(--text-secondary)" }}>
          Sign in
        </Link>
      </div>
    </nav>
  );
}
