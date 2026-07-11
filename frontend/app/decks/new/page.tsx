"use client";

import { useEffect, useRef, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import {
  apiGet,
  apiGetOptional,
  apiPost,
  type CubeView,
  type PublicUser,
  type InferResult,
  type DecklistDetail,
} from "@/lib/api";
import { ARCHETYPES } from "@/lib/decklist";
import { ColorPips } from "@/components/ColorPips";

export default function NewDeckPage() {
  const router = useRouter();
  const [me, setMe] = useState<PublicUser | null | undefined>(undefined);
  const [cubes, setCubes] = useState<CubeView[]>([]);
  const [cubeId, setCubeId] = useState("");
  const [name, setName] = useState("");
  const [archetype, setArchetype] = useState("");
  const [raw, setRaw] = useState("");
  // Optional record, if the deck has already been played.
  const [wins, setWins] = useState("");
  const [losses, setLosses] = useState("");
  const [infer, setInfer] = useState<InferResult | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const timer = useRef<ReturnType<typeof setTimeout>>();

  useEffect(() => {
    apiGetOptional<PublicUser>("/auth/me").then((u) => setMe(u));
    apiGet<CubeView[]>("/cubes")
      .then((cs) => {
        setCubes(cs);
        if (cs[0]) setCubeId(cs[0].cube.id);
      })
      .catch(() => setCubes([]));
  }, []);

  // Debounced live color inference as the list is typed.
  useEffect(() => {
    if (!cubeId || raw.trim() === "") {
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
      };
      const w = parseInt(wins, 10) || 0;
      const l = parseInt(losses, 10) || 0;
      if (w || l) {
        body.wins = w;
        body.losses = l;
      }
      const detail = await apiPost<DecklistDetail>("/decklists", body);
      router.push(`/decks/${detail.decklist.id}`);
    } catch (e) {
      setErr(String(e instanceof Error ? e.message : e));
      setBusy(false);
    }
  }

  if (me === undefined) return <main className="container">Loading…</main>;
  if (me === null) {
    return (
      <main className="container">
        <h1>New deck</h1>
        <p>
          You need to <Link href="/login">sign in</Link> to upload a deck.
        </p>
      </main>
    );
  }

  return (
    <main className="container" style={{ maxWidth: 760 }}>
      <h1>New deck</h1>
      <form onSubmit={submit} className="card">
        <label htmlFor="cube">Cube</label>
        <select id="cube" value={cubeId} onChange={(e) => setCubeId(e.target.value)} required>
          {cubes.map((c) => (
            <option key={c.cube.id} value={c.cube.id}>
              {c.cube.name}
            </option>
          ))}
        </select>

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
          <div
            className="card"
            style={{ marginTop: "0.75rem", display: "flex", alignItems: "center", gap: 10 }}
          >
            <ColorPips bits={infer.color_identity} showCode />
            <span className="muted">
              {(infer.resolved?.length ?? 0)} resolved
              {infer.unresolved && infer.unresolved.length > 0 && (
                <> · {infer.unresolved.length} unresolved: {infer.unresolved.slice(0, 5).join(", ")}
                  {infer.unresolved.length > 5 ? "…" : ""}</>
              )}
            </span>
          </div>
        )}

        <label style={{ marginTop: "1rem" }}>Record (optional — if already played)</label>
        <div style={{ display: "flex", gap: "0.75rem", flexWrap: "wrap" }}>
          <div>
            <span className="muted" style={{ fontSize: "0.8rem" }}>Wins</span>
            <input type="number" min={0} value={wins} onChange={(e) => setWins(e.target.value)} style={{ width: 90 }} />
          </div>
          <div>
            <span className="muted" style={{ fontSize: "0.8rem" }}>Losses</span>
            <input type="number" min={0} value={losses} onChange={(e) => setLosses(e.target.value)} style={{ width: 90 }} />
          </div>
        </div>

        {err && <p style={{ color: "var(--bad)", marginTop: "0.75rem" }}>{err}</p>}

        <button className="button" style={{ marginTop: "1rem" }} disabled={busy}>
          {busy ? "Creating…" : "Create deck"}
        </button>
      </form>
      <p className="muted" style={{ marginTop: "1rem", fontSize: "0.85rem" }}>
        Leave the record blank if you haven&apos;t played yet — you can add it later from the deck page.
      </p>
    </main>
  );
}
