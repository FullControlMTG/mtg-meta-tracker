"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import {
  apiGet,
  apiGetOptional,
  apiPost,
  apiPatch,
  apiDelete,
  type Combo,
  type CubeCard,
  type CubeView,
} from "@/lib/api";
import { useSession } from "@/components/SessionProvider";

// A combo is at least a pair; the ceiling mirrors the backend's, which exists so
// one row cannot swallow a whole archetype's worth of cards.
const MIN_PIECES = 2;
const MAX_PIECES = 10;

// Decklists write "Front" for a card the pool stores as "Front // Back", so a
// piece typed either way has to find the same card (mirrors store.FrontFace).
function frontFace(name: string): string {
  const i = name.indexOf("/");
  return i >= 0 ? name.slice(0, i).trim() : name;
}

// Every name a pool card answers to, lower-cased, so what the admin typed can be
// looked up directly.
function poolIndex(cards: CubeCard[]): Map<string, CubeCard> {
  const idx = new Map<string, CubeCard>();
  for (const c of cards) {
    idx.set(c.card_name.toLowerCase(), c);
    idx.set(frontFace(c.card_name).toLowerCase(), c);
  }
  return idx;
}

export default function AdminCombosPage() {
  const { me } = useSession();
  const [cubes, setCubes] = useState<CubeView[]>([]);
  const [cubeId, setCubeId] = useState("");
  const [pool, setPool] = useState<CubeCard[]>([]);
  const [combos, setCombos] = useState<Combo[]>([]);

  // Form state (create when editingId is null, otherwise update).
  const [editingId, setEditingId] = useState<string | null>(null);
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [pieces, setPieces] = useState<string[]>(["", ""]);
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    apiGet<CubeView[]>("/cubes")
      .then((cs) => {
        setCubes(cs);
        if (cs[0]) setCubeId(cs[0].cube.id);
      })
      .catch(() => setCubes([]));
  }, []);

  // The pool is fetched whole: it is what the piece inputs autocomplete against,
  // and a cube is a few hundred cards — one request beats a search endpoint.
  useEffect(() => {
    if (!cubeId) return;
    resetForm();
    apiGetOptional<CubeCard[]>(`/cubes/${cubeId}/cards`).then((cs) => setPool(cs ?? []));
    refreshCombos(cubeId);
  }, [cubeId]);

  function refreshCombos(id: string) {
    apiGetOptional<Combo[]>(`/cubes/${id}/combos`).then((cs) => setCombos(cs ?? []));
  }

  function resetForm() {
    setEditingId(null);
    setName("");
    setDescription("");
    setPieces(["", ""]);
    setErr(null);
  }

  function startEdit(combo: Combo) {
    setEditingId(combo.id);
    setName(combo.name);
    setDescription(combo.description ?? "");
    setPieces(combo.cards.map((c) => c.card_name));
    setErr(null);
    window.scrollTo({ top: 0, behavior: "smooth" });
  }

  function setPiece(i: number, value: string) {
    setPieces((p) => p.map((v, j) => (j === i ? value : v)));
  }

  function removePiece(i: number) {
    setPieces((p) => (p.length <= MIN_PIECES ? p.map((v, j) => (j === i ? "" : v)) : p.filter((_, j) => j !== i)));
  }

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setErr(null);

    // Names are resolved against the pool here rather than server-side: the form
    // already holds the cards, and an unmatched name is worth naming back to the
    // admin before anything is saved.
    const idx = poolIndex(pool);
    const typed = pieces.map((p) => p.trim()).filter((p) => p !== "");
    const missing = typed.filter((p) => !idx.has(p.toLowerCase()));
    if (missing.length > 0) {
      setErr(`Not in this cube's pool: ${missing.join(", ")}`);
      return;
    }
    const cardIds = Array.from(new Set(typed.map((p) => idx.get(p.toLowerCase())!.card_id)));
    if (cardIds.length < MIN_PIECES) {
      setErr("A combo needs at least two different cards.");
      return;
    }

    setBusy(true);
    try {
      const body = { name, description, card_ids: cardIds };
      if (editingId) {
        await apiPatch<Combo>(`/admin/combos/${editingId}`, body);
      } else {
        await apiPost<Combo>(`/admin/cubes/${cubeId}/combos`, body);
      }
      resetForm();
      refreshCombos(cubeId);
    } catch (e) {
      setErr(String(e instanceof Error ? e.message : e));
    } finally {
      setBusy(false);
    }
  }

  async function remove(combo: Combo) {
    if (!window.confirm(`Delete combo "${combo.name}"? Decks will stop reporting it.`)) return;
    try {
      await apiDelete(`/admin/combos/${combo.id}`);
      if (editingId === combo.id) resetForm();
      refreshCombos(cubeId);
    } catch (e) {
      setErr(String(e instanceof Error ? e.message : e));
    }
  }

  if (me === undefined) return <main className="container">Loading…</main>;
  if (me === null || me.role !== "admin") {
    return (
      <main className="container">
        <h1>Combos</h1>
        <p>
          You are not authorized to view this page. <Link href="/">Go home</Link>.
        </p>
      </main>
    );
  }

  return (
    <main className="container" style={{ maxWidth: 820 }}>
      <h1>Combos</h1>
      <p className="muted" style={{ marginTop: "-0.5rem" }}>
        Name a set of cards that play together. Any deck whose mainboard holds all of them
        lists the combo on its page — a sub-archetype the colour and archetype tags cannot
        express.
      </p>

      <label htmlFor="cube">Cube</label>
      <select id="cube" value={cubeId} onChange={(e) => setCubeId(e.target.value)}>
        {cubes.map((c) => (
          <option key={c.cube.id} value={c.cube.id}>
            {c.cube.name}
          </option>
        ))}
      </select>

      {/* One shared list for every piece input; the pool is the only legal source
          of pieces, so the browser's own autocomplete is the whole picker. */}
      <datalist id="pool-cards">
        {pool.map((c) => (
          <option key={c.card_id} value={c.card_name} />
        ))}
      </datalist>

      <form onSubmit={submit} className="card" style={{ marginTop: "1rem" }}>
        <h2 style={{ marginTop: 0 }}>{editingId ? "Edit combo" : "New combo"}</h2>

        <label htmlFor="name">Name</label>
        <input
          id="name"
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="Thoracle"
          required
        />

        <label htmlFor="desc">Description (optional)</label>
        <input
          id="desc"
          value={description}
          onChange={(e) => setDescription(e.target.value)}
          placeholder="Empty your library, then win on the Oracle trigger"
        />

        <label>Cards</label>
        <div style={{ display: "flex", flexDirection: "column", gap: "0.5rem" }}>
          {pieces.map((p, i) => (
            <div key={i} style={{ display: "flex", gap: "0.5rem" }}>
              <input
                list="pool-cards"
                value={p}
                onChange={(e) => setPiece(i, e.target.value)}
                placeholder={`Card ${i + 1}`}
                aria-label={`Card ${i + 1}`}
              />
              <button
                type="button"
                className="button"
                onClick={() => removePiece(i)}
                aria-label={`Remove card ${i + 1}`}
                style={{
                  background: "var(--surface)",
                  color: "var(--text)",
                  border: "1px solid var(--border)",
                  flexShrink: 0,
                }}
              >
                ✕
              </button>
            </div>
          ))}
        </div>
        <button
          type="button"
          className="button"
          onClick={() => setPieces((ps) => [...ps, ""])}
          disabled={pieces.length >= MAX_PIECES}
          style={{
            marginTop: "0.5rem",
            background: "var(--surface)",
            color: "var(--text)",
            border: "1px solid var(--border)",
          }}
        >
          + Add card
        </button>
        <p className="muted" style={{ margin: "0.5rem 0 0", fontSize: "0.8rem" }}>
          Type a card from this cube&apos;s pool — the field autocompletes. Two cards minimum,
          {" "}
          {MAX_PIECES} maximum.
        </p>

        {err && <p style={{ color: "var(--bad)", marginTop: "0.75rem" }}>{err}</p>}

        <div style={{ display: "flex", gap: "0.75rem", marginTop: "1rem" }}>
          <button className="button" disabled={busy || !cubeId}>
            {busy ? "Saving…" : editingId ? "Save changes" : "Create combo"}
          </button>
          {editingId && (
            <button
              type="button"
              className="button"
              onClick={resetForm}
              style={{ background: "var(--surface)", color: "var(--text)", border: "1px solid var(--border)" }}
            >
              Cancel
            </button>
          )}
        </div>
      </form>

      <h2>Configured combos</h2>
      {combos.length === 0 && <p className="muted">No combos in this cube yet.</p>}
      <div style={{ display: "flex", flexDirection: "column", gap: "0.75rem" }}>
        {combos.map((combo) => (
          <div key={combo.id} className="card">
            <strong style={{ fontSize: "1.05rem" }}>{combo.name}</strong>
            <p style={{ margin: "0.25rem 0" }}>{combo.cards.map((c) => c.card_name).join(" + ")}</p>
            {combo.description && (
              <p className="muted" style={{ margin: "0.25rem 0", fontSize: "0.85rem" }}>
                {combo.description}
              </p>
            )}
            <div style={{ display: "flex", gap: "0.75rem", marginTop: "0.5rem", flexWrap: "wrap" }}>
              <button type="button" className="button" onClick={() => startEdit(combo)}>
                Edit
              </button>
              <button
                type="button"
                className="button"
                onClick={() => remove(combo)}
                style={{ background: "var(--bad, #b00)", color: "#fff" }}
              >
                Delete
              </button>
            </div>
          </div>
        ))}
      </div>
    </main>
  );
}
