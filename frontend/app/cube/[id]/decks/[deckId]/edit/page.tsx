"use client";

import { useEffect, useRef, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import {
  apiGetOptional,
  apiPost,
  apiPatch,
  apiDelete,
  type CubeView,
  type DecklistDetail,
  type PublicUser,
  type InferResult,
} from "@/lib/api";
import { cubePath } from "@/lib/cube";
import { ARCHETYPES, STATUSES } from "@/lib/decklist";
import { isoDay } from "@/lib/format";
import { ColorPips } from "@/components/ColorPips";

export default function EditDeckPage({ params }: { params: { id: string; deckId: string } }) {
  const router = useRouter();
  const cubeId = params.id;
  const deckId = params.deckId;

  const [me, setMe] = useState<PublicUser | null | undefined>(undefined);
  const [detail, setDetail] = useState<DecklistDetail | null | undefined>(undefined);

  // Deck fields.
  const [name, setName] = useState("");
  const [archetype, setArchetype] = useState("");
  const [status, setStatus] = useState("active");
  const [playedAt, setPlayedAt] = useState("");
  const [raw, setRaw] = useState("");
  // Owner reassignment is offered to the cube owner / an admin; the list is members.
  const [members, setMembers] = useState<PublicUser[]>([]);
  const [canAssign, setCanAssign] = useState(false);
  const [userId, setUserId] = useState("");
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

  // Delete.
  const [delErr, setDelErr] = useState<string | null>(null);
  const [delBusy, setDelBusy] = useState(false);

  useEffect(() => {
    Promise.all([
      apiGetOptional<PublicUser>("/auth/me"),
      apiGetOptional<CubeView>(`/cubes/${cubeId}`),
      apiGetOptional<PublicUser[]>(`/cubes/${cubeId}/members`),
    ]).then(([u, view, ms]) => {
      setMe(u);
      setMembers(ms ?? []);
      if (u && (u.role === "admin" || u.id === view?.cube.owner_id)) setCanAssign(true);
    });
    apiGetOptional<DecklistDetail>(`/decklists/${deckId}`).then((dd) => {
      setDetail(dd);
      if (dd) {
        const d = dd.decklist;
        setName(d.name);
        setArchetype(d.archetype ?? "");
        setStatus(d.status);
        setPlayedAt(isoDay(d.played_at));
        setRaw(d.decklist_raw);
        setUserId(d.user_id);
        if (d.games_played > 0 || d.wins || d.losses) {
          setWins(String(d.wins));
          setLosses(String(d.losses));
        }
      }
    });
  }, [cubeId, deckId]);

  // Debounced live color inference as the list is edited.
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

  async function saveDeck(e: React.FormEvent) {
    e.preventDefault();
    setDeckBusy(true);
    setDeckErr(null);
    setDeckMsg(null);
    try {
      const body: Record<string, unknown> = { name, archetype, status, played_at: playedAt, decklist_raw: raw };
      if (canAssign && userId) body.user_id = userId;
      const saved = await apiPatch<DecklistDetail>(`/decklists/${deckId}`, body);
      setDetail(saved);
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
      await apiPatch<DecklistDetail>(`/decklists/${deckId}/record`, { wins: w, losses: l });
      setRecMsg("Record saved.");
      router.refresh();
    } catch (e) {
      setRecErr(String(e instanceof Error ? e.message : e));
    } finally {
      setRecBusy(false);
    }
  }

  async function deleteDeck() {
    const deckName = detail?.decklist.name ?? "this deck";
    if (!window.confirm(`Delete "${deckName}"? Its record and card list go with it. This cannot be undone.`)) {
      return;
    }
    setDelBusy(true);
    setDelErr(null);
    try {
      await apiDelete(`/decklists/${deckId}`);
      router.push(cubePath(cubeId, "/decks"));
    } catch (e) {
      setDelErr(String(e instanceof Error ? e.message : e));
      setDelBusy(false);
    }
  }

  if (me === undefined || detail === undefined) return <p style={{ marginTop: "1rem" }}>Loading…</p>;
  if (detail === null) {
    return (
      <p style={{ marginTop: "1rem" }}>
        Deck not found. <Link href={cubePath(cubeId, "/decks")}>Back to decks</Link>.
      </p>
    );
  }
  // Mirrors the backend's CanMutateOwned: the owner, or any admin.
  if (!me || (me.id !== detail.decklist.user_id && me.role !== "admin")) {
    return (
      <p style={{ marginTop: "1rem" }}>
        You are not allowed to edit this deck.{" "}
        <Link href={cubePath(cubeId, `/decks/${deckId}`)}>View deck</Link>.
      </p>
    );
  }

  return (
    <div style={{ maxWidth: 760, marginTop: "1rem" }}>
      <p className="muted" style={{ marginBottom: "0.25rem" }}>
        <Link href={cubePath(cubeId, `/decks/${deckId}`)}>← {detail.decklist.name}</Link>
      </p>
      <h2 style={{ marginTop: 0 }}>Edit deck</h2>

      <form onSubmit={saveDeck} className="card">
        <h3 style={{ marginTop: 0 }}>Deck</h3>

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

        <label htmlFor="status">Status</label>
        <select id="status" value={status} onChange={(e) => setStatus(e.target.value)}>
          {STATUSES.map((s) => (
            <option key={s} value={s}>
              {s}
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

        {deckErr && <p style={{ color: "var(--bad)", marginTop: "0.75rem" }}>{deckErr}</p>}
        {deckMsg && <p className="muted" style={{ marginTop: "0.75rem" }}>{deckMsg}</p>}

        <button className="button" style={{ marginTop: "1rem" }} disabled={deckBusy}>
          {deckBusy ? "Saving…" : "Save deck"}
        </button>
      </form>

      <form onSubmit={saveRecord} className="card" id="record">
        <h3 style={{ marginTop: 0 }}>Record</h3>
        <p className="muted" style={{ marginTop: 0, fontSize: "0.85rem" }}>
          Add or update your win/loss record. Games played is the sum of wins and losses.
        </p>
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

        {recErr && <p style={{ color: "var(--bad)", marginTop: "0.75rem" }}>{recErr}</p>}
        {recMsg && <p className="muted" style={{ marginTop: "0.75rem" }}>{recMsg}</p>}

        <button className="button" style={{ marginTop: "1rem" }} disabled={recBusy}>
          {recBusy ? "Saving…" : "Save record"}
        </button>
      </form>

      <div className="card">
        <h3 style={{ marginTop: 0 }}>Danger zone</h3>
        <p className="muted" style={{ marginTop: 0, fontSize: "0.85rem" }}>
          Deleting removes the deck, its card list, and its win/loss record for good. To take it out
          of circulation without losing it, set its status to archived instead.
        </p>

        {delErr && <p style={{ color: "var(--bad)", marginTop: "0.75rem" }}>{delErr}</p>}

        <button
          type="button"
          className="button"
          onClick={deleteDeck}
          disabled={delBusy}
          style={{ marginTop: "1rem", background: "var(--bad, #b00)", color: "#fff" }}
        >
          {delBusy ? "Deleting…" : "Delete deck"}
        </button>
      </div>
    </div>
  );
}
