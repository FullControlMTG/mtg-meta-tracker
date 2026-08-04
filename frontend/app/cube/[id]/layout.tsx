import Link from "next/link";
import { notFound } from "next/navigation";
import { apiGetOptional, type CubeView, type PublicUser } from "@/lib/api";
import { getCubes, cubePath } from "@/lib/cube";
import { CubeSwitcher } from "@/components/CubeSwitcher";
import { CubeTabs } from "@/components/CubeTabs";

// The chrome shared by every page inside a cube: the cube's name, a switcher, and the
// tab bar. Fetching the cube here is also the access gate — the backend 404s a
// non-member, so apiGetOptional returns null and we notFound(), which is what keeps a
// cube you don't belong to out of reach.
export default async function CubeLayout({
  children,
  params,
}: {
  children: React.ReactNode;
  params: { id: string };
}) {
  const [view, cubes, me] = await Promise.all([
    apiGetOptional<CubeView>(`/cubes/${params.id}`, 0),
    getCubes(0),
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
        <div style={{ display: "flex", alignItems: "center", gap: "0.75rem", flexWrap: "wrap" }}>
          {showManage && (
            <Link href={cubePath(params.id, "/manage")} className="button">
              Manage cube
            </Link>
          )}
          <CubeSwitcher
            cubes={cubes.map((c) => ({ id: c.cube.id, name: c.cube.name }))}
            current={params.id}
          />
        </div>
      </div>

      <CubeTabs cubeId={params.id} />

      {children}
    </div>
  );
}
