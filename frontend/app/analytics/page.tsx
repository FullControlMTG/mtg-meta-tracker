import { redirect } from "next/navigation";
import { getDefaultCube } from "@/lib/cube";

// Bare /analytics has no meaning — stats belong to a cube. Land on the first one.
export const dynamic = "force-dynamic";

export default async function AnalyticsIndex() {
  const cube = await getDefaultCube(0);
  if (cube) redirect(`/analytics/${cube.cube.id}`);

  return (
    <main className="container">
      <h1>Analytics</h1>
      <p className="muted">No cube configured yet. An admin can add one.</p>
    </main>
  );
}
