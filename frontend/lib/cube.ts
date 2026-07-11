import { apiGet, type CubeView } from "@/lib/api";

// Cubes are stable; longer revalidate is fine. Pages that render per request pass
// 0 to opt out of caching entirely — see apiGet.
export async function getCubes(revalidate = 300): Promise<CubeView[]> {
  try {
    return await apiGet<CubeView[]>("/cubes", revalidate);
  } catch (e) {
    console.warn("GET /cubes failed; rendering with no cubes", e);
    return [];
  }
}

// The playgroup runs a small number of cubes; default to the first.
export async function getDefaultCube(revalidate = 300): Promise<CubeView | null> {
  const cubes = await getCubes(revalidate);
  return cubes[0] ?? null;
}
