import Link from "next/link";
import { getCubes } from "@/lib/cube";

export const revalidate = 300;

function fmtDate(s?: string): string {
  if (!s) return "never";
  const d = new Date(s);
  return isNaN(d.getTime()) ? "—" : d.toLocaleDateString();
}

export default async function CubesPage() {
  const cubes = await getCubes();

  return (
    <main className="container">
      <h1>Cubes</h1>
      <p className="muted">Browse the cards in each cube.</p>

      {cubes.length === 0 ? (
        <p className="muted">No cubes yet.</p>
      ) : (
        <div className="card" style={{ marginTop: "1rem", overflowX: "auto" }}>
          <table>
            <thead>
              <tr>
                <th>Cube</th>
                <th className="num">Cards</th>
                <th>Last synced</th>
              </tr>
            </thead>
            <tbody>
              {cubes.map((cv) => (
                <tr key={cv.cube.id}>
                  <td>
                    <Link href={`/cubes/${cv.cube.id}`}>{cv.cube.name}</Link>
                    {cv.cube.description && (
                      <span className="muted" style={{ marginLeft: 6, fontSize: "0.85rem" }}>
                        {cv.cube.description}
                      </span>
                    )}
                  </td>
                  <td className="num">{cv.card_count}</td>
                  <td className="muted">{fmtDate(cv.cube.last_synced_at)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </main>
  );
}
