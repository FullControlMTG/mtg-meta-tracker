import { redirect } from "next/navigation";
import { getDefaultCube } from "@/lib/cube";

// All stats are cube-scoped, so the landing page is just the first cube's stats.
export const dynamic = "force-dynamic";

export default async function Home() {
  const cube = await getDefaultCube(0);
  if (cube) redirect(`/analytics/${cube.cube.id}`);

  return (
    <main className="container">
      <h1>Meta Tracker</h1>
      <p className="muted">No cube configured yet. An admin can add one.</p>
    </main>
  );
}
