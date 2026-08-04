import Link from "next/link";
import { notFound } from "next/navigation";
import { apiGetOptional, type CubeView, type PublicUser } from "@/lib/api";
import { cubePath } from "@/lib/cube";
import { CubeTabs } from "@/components/CubeTabs";

// The chrome shared by every page inside a cube: the cube's name, a Manage button (for
// owner/admin), and the tab bar. Switching cubes is done from the dashboard now, not a
// header dropdown. Fetching the cube here is also the access gate — the backend 404s a
// non-member, so apiGetOptional returns null and we notFound().
export default async function CubeLayout({
  children,
  params,
}: {
  children: React.ReactNode;
  params: { id: string };
}) {
  const [view, me] = await Promise.all([
    apiGetOptional<CubeView>(`/cubes/${params.id}`, 0),
    apiGetOptional<PublicUser>("/auth/me", 0),
  ]);
  if (!view) notFound();

  const showManage = me?.role === "admin" || (me?.id != null && me.id === view.cube.owner_id);

  return (
    <div className="container">
      <div
        style={{
          display: "flex",
          alignItems: "baseline",
          justifyContent: "space-between",
          gap: "1rem",
          flexWrap: "wrap",
        }}
      >
        <div>
          <p className="muted" style={{ margin: "0 0 0.15rem" }}>
            <Link href="/">← Dashboard</Link>
          </p>
          <h1 style={{ margin: 0 }}>{view.cube.name}</h1>
        </div>
        {showManage && (
          <Link href={cubePath(params.id, "/manage")} className="button">
            Manage cube
          </Link>
        )}
      </div>

      <CubeTabs cubeId={params.id} />

      {children}
    </div>
  );
}
