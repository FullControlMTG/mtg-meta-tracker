import { apiGet, type CubeView } from "@/lib/api";

// Cubes are stable; longer revalidate is fine.
export async function getCubes(): Promise<CubeView[]> {
  try {
    return await apiGet<CubeView[]>("/cubes", 300);
  } catch {
    return [];
  }
}

// The playgroup runs a small number of cubes; default to the first.
export async function getDefaultCube(): Promise<CubeView | null> {
  const cubes = await getCubes();
  return cubes[0] ?? null;
}
