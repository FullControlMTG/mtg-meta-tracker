import { revalidatePath } from "next/cache";
import { NextRequest, NextResponse } from "next/server";

// Handled here rather than proxied: this route takes precedence over the /api rewrite.
export async function POST(req: NextRequest) {
  const secret = req.headers.get("x-revalidate-secret");
  if (!secret || secret !== process.env.REVALIDATE_SECRET) {
    return NextResponse.json({ error: "unauthorized" }, { status: 401 });
  }
  const { paths } = (await req.json()) as { paths?: string[] };
  for (const p of paths ?? []) {
    revalidatePath(p);
  }
  return NextResponse.json({ revalidated: paths ?? [] });
}
