"use client";

import { useEffect, useRef, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import {
  apiGetOptional,
  apiPost,
  type CubeView,
  type PublicUser,
  type InferResult,
  type DecklistDetail,
  type Today,
} from "@/lib/api";
import { cubePath } from "@/lib/cube";
import { ARCHETYPES } from "@/lib/decklist";
import { ColorPips } from "@/components/ColorPips";

export default function NewDeckPage({ params }: { params: { id: string } }) {
  const router = useRouter();
  const cubeId = params.id;
  const [me, setMe] = useState<PublicUser | null | undefined>(undefined);
  // The owner picker is offered only to the cube owner / an admin (mirrors the
  // backend resolveOwner); it lists the cube's members, never all users.
  const [members, setMembers] = useState<PublicUser[]>([]);
  const [canAssign, setCanAssign] = useState(false);
  const [userId, setUserId] = useState("");
  const [name, setName] = useState("");
  const [archetype, setArchetype] = useState("");
  const [playedAt, setPlayedAt] = useState("");
  const [raw, setRaw] = useState("");
  const [wins, setWins] = useState("");
  const [losses, setLosses] = useState("");
  const [infer, setInfer] = useState<InferResult | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const timer = useRef<ReturnType<typeof setTimeout>>();

  useEffect(() => {
    apiGetOptional<Today>("/today").then((t) => t && setPlayedAt(t.date));
    Promise.all([
      apiGetOptional<PublicUser>("/auth/me"),
      apiGetOptional<CubeView>(`/cubes/${cubeId}`),
      apiGetOptional<PublicUser[]>(`/cubes/${cubeId}/members`),
    ]).then(([u, view, ms]) => {
      setMe(u);
      if (u) setUserId(u.id);
      setMembers(ms ?? []);
      if (u && (u.role === "admin" || u.id === view?.cube.owner_id)) setCanAssign(true);
    });
  }, [cubeId]);

  // Debounced live color inference as the list is typed.
  useEffect(() => {
    if (raw.trim() === "") {
      setInfer(null);
      return;
    }
    clearTimeout(timer.current);
    timer.current = setTimeout(() => {
      apiPost<InferResult>("/decklists/infer-colors", { cube_id: cubeId, decklist_raw: raw })
        .then(setInfer)
        .catch(() => setInfer(null));
    }, 400);
    return () => clearTimeout(timer.current);
  }, [raw, cubeId]);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setErr(null);
    try {
      const body: Record<string, unknown> = {
        cube_id: cubeId,
        name,
        archetype,
        decklist_raw: raw,
        played_at: playedAt,
      };
      if (canAssign && userId) body.user_id = userId;
      const w = parseInt(wins, 10) || 0;
      const l = parseInt(losses, 10) || 0;
      if (w || l) {
        body.wins = w;
        body.losses = l;
      }
      const detail = await apiPost<DecklistDetail>("/decklists", body);
      router.push(cubePath(cubeId, `/decks/${detail.decklist.id}`));
    } catch (e) {
      setErr(String(e instanceof Error ? e.message : e));
      setBusy(false);
    }
  }

  if (me === undefined) return <p style={{ marginTop: "1rem" }}>Loading…</p>;
  if (me === null) {
    return (
      <p style={{ marginTop: "1rem" }}>
        You need to <Link href="/login">sign in</Link> to upload a deck.
      </p>
    );
  }

  return (
    <div style={{ maxWidth: 760, marginTop: "1rem" }}>
      <h2 style={{ marginTop: 0 }}>New deck</h2>
      <form onSubmit={submit} className="card">
        {canAssign && (
          <>
            <label htmlFor="owner">Owner</label>
            <select id="owner" value={userId} onChange={(e) => setUserId(e.target.value)} required>
              {members.map((u) => (
                <option key={u.id} value={u.id}>
                  {u.display_name} (@{u.username})
                </option>
              ))}
            </select>
          </>
        )}

        <label htmlFor="name">Deck name</label>
        <input id="name" value={name} onChange={(e) => setName(e.target.value)} required />

        <label htmlFor="archetype">Archetype (optional)</label>
        <select id="archetype" value={archetype} onChange={(e) => setArchetype(e.target.value)}>
          <option value="">— none —</option>
          {ARCHETYPES.map((a) => (
            <option key={a} value={a}>
              {a}
            </option>
          ))}
        </select>

        <label htmlFor="played">Date played</label>
        <input id="played" type="date" value={playedAt} onChange={(e) => setPlayedAt(e.target.value)} required />

        <label htmlFor="list">Decklist</label>
        <textarea
          id="list"
          value={raw}
          onChange={(e) => setRaw(e.target.value)}
          rows={12}
          placeholder={"1 Lightning Bolt\n1 Sol Ring\n…"}
          required
          style={{ fontFamily: "ui-monospace, monospace", resize: "vertical" }}
        />

        {infer && (
          <div className="card" style={{ marginTop: "0.75rem" }}>
            <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
              <ColorPips bits={infer.color_identity} splash={infer.splash_colors} showCode />
              <span className="muted">
                {infer.resolved?.length ?? 0} resolved
                {infer.unresolved && infer.unresolved.length > 0 && (
                  <>
                    {" "}
                    · {infer.unresolved.length} unresolved: {infer.unresolved.slice(0, 5).join(", ")}
                    {infer.unresolved.length > 5 ? "…" : ""}
                  </>
                )}
              </span>
            </div>
            {infer.combos?.length > 0 && (
              <p className="muted" style={{ margin: "0.5rem 0 0", fontSize: "0.85rem" }}>
                Combos: {infer.combos.map((c) => c.name).join(", ")}
              </p>
            )}
          </div>
        )}

        <label style={{ marginTop: "1rem" }}>Record (optional — if already played)</label>
        <div style={{ display: "flex", gap: "0.75rem", flexWrap: "wrap" }}>
          <div>
            <span className="muted" style={{ fontSize: "0.8rem" }}>
              Wins
            </span>
            <input type="number" min={0} value={wins} onChange={(e) => setWins(e.target.value)} style={{ width: 90 }} />
          </div>
          <div>
            <span className="muted" style={{ fontSize: "0.8rem" }}>
              Losses
            </span>
            <input type="number" min={0} value={losses} onChange={(e) => setLosses(e.target.value)} style={{ width: 90 }} />
          </div>
        </div>

        {err && <p style={{ color: "var(--bad)", marginTop: "0.75rem" }}>{err}</p>}

        <button className="button" style={{ marginTop: "1rem" }} disabled={busy}>
          {busy ? "Creating…" : "Create deck"}
        </button>
      </form>
      <p className="muted" style={{ marginTop: "1rem", fontSize: "0.85rem" }}>
        Leave the record blank if you haven&apos;t played yet — you can add it later from the deck
        page.
      </p>
    </div>
  );
}
