import { redirect } from "next/navigation";
import Link from "next/link";
import { apiGetOptional, type PublicUser } from "@/lib/api";

// Server-side gate for the whole /admin/* tree. The /auth/me fetch is the backend
// check — a non-admin (or anon) is redirected before any admin page HTML is produced.
// The admin *actions* are independently enforced by requireAdmin on the API, so this is
// the page-level half of a defense in depth, not the only guard.
export default async function AdminLayout({ children }: { children: React.ReactNode }) {
  const me = await apiGetOptional<PublicUser>("/auth/me", 0);
  if (!me || me.role !== "admin") redirect("/");

  return (
    <>
      <div className="container" style={{ paddingBottom: 0 }}>
        <p className="muted" style={{ margin: "0.5rem 0 0" }}>
          <Link href="/admin">← Admin</Link>
        </p>
      </div>
      {children}
    </>
  );
}
