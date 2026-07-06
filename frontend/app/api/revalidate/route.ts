import { revalidatePath } from "next/cache";
import { NextRequest, NextResponse } from "next/server";

// Called by the Go backend after a recompute so static decklist/user/analytics
// pages re-render on real change only. This app/api route takes precedence over
// the afterFiles /api proxy rewrite, so it is handled here, not forwarded.
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
