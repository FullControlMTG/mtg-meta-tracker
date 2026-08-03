import { apiGet, type CubeView } from "@/lib/api";

// The cubes the caller belongs to (the backend scopes /cubes to membership).
export async function getCubes(revalidate = 300): Promise<CubeView[]> {
  try {
    return await apiGet<CubeView[]>("/cubes", revalidate);
  } catch (e) {
    console.warn("GET /cubes failed; rendering with no cubes", e);
    return [];
  }
}

// Every cube-scoped path hangs off /cube/<id>; build them here so the prefix lives in
// one place. sub is "", "/cards", "/decks/new", etc.
export function cubePath(cubeId: string, sub = ""): string {
  return `/cube/${cubeId}${sub}`;
}
