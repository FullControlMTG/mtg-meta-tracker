import Link from "next/link";

// The admin home: one box per admin area. Add a section by adding an entry here plus its
// page under app/admin/…; the grid grows to fit.
const sections = [
  {
    href: "/admin/users/manage",
    title: "Manage users",
    desc: "Change roles, delete accounts, reset passwords.",
  },
  {
    href: "/admin/users/add",
    title: "Add user",
    desc: "Create an account and hand over its first password.",
  },
];

export default function AdminHome() {
  return (
    <main className="container">
      <h1>Admin</h1>
      <div
        className="grid"
        style={{ marginTop: "1rem", gridTemplateColumns: "repeat(auto-fill, minmax(240px, 1fr))", gap: "0.75rem" }}
      >
        {sections.map((s) => (
          <Link key={s.href} href={s.href} className="card" style={{ textDecoration: "none" }}>
            <strong style={{ fontSize: "1.05rem" }}>{s.title}</strong>
            <p className="muted" style={{ margin: "0.25rem 0 0", fontSize: "0.85rem" }}>
              {s.desc}
            </p>
          </Link>
        ))}
      </div>
    </main>
  );
}
