import { getCubes } from "@/lib/cube";
import { AnalyticsDashboard } from "@/components/AnalyticsDashboard";

// Interactive, client-fetched; render the shell fresh each load.
export const dynamic = "force-dynamic";

export default async function AnalyticsPage() {
  const cubes = await getCubes();
  if (cubes.length === 0) {
    return (
      <main className="container">
        <h1>Analytics</h1>
        <p className="muted">No cube configured yet.</p>
      </main>
    );
  }
  return (
    <main className="container">
      <h1>Analytics</h1>
      <AnalyticsDashboard
        cubes={cubes.map((c) => ({ id: c.cube.id, name: c.cube.name }))}
      />
    </main>
  );
}
