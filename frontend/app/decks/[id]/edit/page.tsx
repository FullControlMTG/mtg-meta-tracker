"use client";

import { useEffect, useRef, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import {
  apiGetOptional,
  apiPost,
  apiPatch,
  type DecklistDetail,
  type PublicUser,
  type InferResult,
} from "@/lib/api";
import { ARCHETYPES, STATUSES } from "@/lib/decklist";
import { ColorPips } from "@/components/ColorPips";

export default function EditDeckPage({ params }: { params: { id: string } }) {
  const router = useRouter();
  const { id } = params;

  const [me, setMe] = useState<PublicUser | null | undefined>(undefined);
  const [detail, setDetail] = useState<DecklistDetail | null | undefined>(undefined);

  // Deck fields.
  const [name, setName] = useState("");
  const [archetype, setArchetype] = useState("");
  const [status, setStatus] = useState("active");
  const [raw, setRaw] = useState("");
  const [cubeId, setCubeId] = useState("");
  const [infer, setInfer] = useState<InferResult | null>(null);
  const [deckErr, setDeckErr] = useState<string | null>(null);
  const [deckBusy, setDeckBusy] = useState(false);
  const [deckMsg, setDeckMsg] = useState<string | null>(null);
  const timer = useRef<ReturnType<typeof setTimeout>>();

  // Record fields.
  const [wins, setWins] = useState("");
  const [losses, setLosses] = useState("");
  const [recErr, setRecErr] = useState<string | null>(null);
  const [recBusy, setRecBusy] = useState(false);
  const [recMsg, setRecMsg] = useState<string | null>(null);

  useEffect(() => {
    apiGetOptional<PublicUser>("/auth/me").then(setMe);
    apiGetOptional<DecklistDetail>(`/decklists/${id}`).then((dd) => {
      setDetail(dd);
      if (dd) {
        const d = dd.decklist;
        setName(d.name);
        setArchetype(d.archetype ?? "");
        setStatus(d.status);
        setRaw(d.decklist_raw);
        setCubeId(d.cube_id);
        if (d.games_played > 0 || d.wins || d.losses) {
          setWins(String(d.wins));
          setLosses(String(d.losses));
        }
      }
    });
  }, [id]);

  // Debounced live color inference as the list is edited.
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

  async function saveDeck(e: React.FormEvent) {
    e.preventDefault();
    setDeckBusy(true);
    setDeckErr(null);
    setDeckMsg(null);
    try {
      await apiPatch<DecklistDetail>(`/decklists/${id}`, {
        name,
        archetype,
        status,
        decklist_raw: raw,
      });
      setDeckMsg("Saved.");
      router.refresh();
    } catch (e) {
      setDeckErr(String(e instanceof Error ? e.message : e));
    } finally {
      setDeckBusy(false);
    }
  }

  async function saveRecord(e: React.FormEvent) {
    e.preventDefault();
    setRecBusy(true);
    setRecErr(null);
    setRecMsg(null);
    try {
      const w = parseInt(wins, 10) || 0;
      const l = parseInt(losses, 10) || 0;
      await apiPatch<DecklistDetail>(`/decklists/${id}/record`, {
        wins: w,
        losses: l,
      });
      setRecMsg("Record saved.");
      router.refresh();
    } catch (e) {
      setRecErr(String(e instanceof Error ? e.message : e));
    } finally {
      setRecBusy(false);
    }
  }

  if (me === undefined || detail === undefined) return <main className="container">Loading…</main>;
  if (detail === null) {
    return (
      <main className="container">
        <h1>Edit deck</h1>
        <p>Deck not found. <Link href="/decks">Back to decks</Link>.</p>
      </main>
    );
  }
  if (!me || me.id !== detail.decklist.user_id) {
    return (
      <main className="container">
        <h1>Edit deck</h1>
        <p>You are not allowed to edit this deck. <Link href={`/decks/${id}`}>View deck</Link>.</p>
      </main>
    );
  }

  return (
    <main className="container" style={{ maxWidth: 760 }}>
      <p className="muted" style={{ marginBottom: "0.25rem" }}>
        <Link href={`/decks/${id}`}>← {detail.decklist.name}</Link>
      </p>
      <h1>Edit deck</h1>

      <form onSubmit={saveDeck} className="card">
        <h2 style={{ marginTop: 0 }}>Deck</h2>

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

        <label htmlFor="status">Status</label>
        <select id="status" value={status} onChange={(e) => setStatus(e.target.value)}>
          {STATUSES.map((s) => (
            <option key={s} value={s}>{s}</option>
          ))}
        </select>

        <label htmlFor="list">Decklist</label>
        <textarea
          id="list"
          value={raw}
          onChange={(e) => setRaw(e.target.value)}
          rows={12}
          required
          style={{ fontFamily: "ui-monospace, monospace", resize: "vertical" }}
        />

        {infer && (
          <div className="card" style={{ marginTop: "0.75rem", display: "flex", alignItems: "center", gap: 10 }}>
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

        {deckErr && <p style={{ color: "var(--bad)", marginTop: "0.75rem" }}>{deckErr}</p>}
        {deckMsg && <p className="muted" style={{ marginTop: "0.75rem" }}>{deckMsg}</p>}

        <button className="button" style={{ marginTop: "1rem" }} disabled={deckBusy}>
          {deckBusy ? "Saving…" : "Save deck"}
        </button>
      </form>

      <form onSubmit={saveRecord} className="card" id="record">
        <h2 style={{ marginTop: 0 }}>Record</h2>
        <p className="muted" style={{ marginTop: 0, fontSize: "0.85rem" }}>
          Add or update your win/loss record. Games played is the sum of wins and losses.
        </p>
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

        {recErr && <p style={{ color: "var(--bad)", marginTop: "0.75rem" }}>{recErr}</p>}
        {recMsg && <p className="muted" style={{ marginTop: "0.75rem" }}>{recMsg}</p>}

        <button className="button" style={{ marginTop: "1rem" }} disabled={recBusy}>
          {recBusy ? "Saving…" : "Save record"}
        </button>
      </form>
    </main>
  );
}
