"use client";

import { useEffect, useRef, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import {
  apiGetOptional,
  apiGetNoStore,
  apiPost,
  apiPatch,
  apiDelete,
  type Combo,
  type CubeCard,
  type CubeInvite,
  type CubeView,
  type CubeSyncStatus,
  type PublicUser,
} from "@/lib/api";
import { cubePath } from "@/lib/cube";
import { useSession } from "@/components/SessionProvider";
import { UserSearch, MemberRow } from "@/components/UserSearch";

// The owner's control panel for one cube: its settings + pool sync, its members and
// invites, and its combos. Owner-or-admin only — the backend gates every action, and
// this page hides them from anyone else.
export default function ManageCubePage({ params }: { params: { id: string } }) {
  const cubeId = params.id;
  const { me, refresh: refreshSession } = useSession();
  const [view, setView] = useState<CubeView | null | undefined>(undefined);

  useEffect(() => {
    apiGetOptional<CubeView>(`/cubes/${cubeId}`).then((v) => setView(v));
  }, [cubeId]);

  if (me === undefined || view === undefined) return <p style={{ marginTop: "1rem" }}>Loading…</p>;
  if (!view) {
    return (
      <p style={{ marginTop: "1rem" }}>
        Cube not found. <Link href="/">Back to the dashboard</Link>.
      </p>
    );
  }
  const allowed = me && (me.role === "admin" || me.id === view.cube.owner_id);
  if (!allowed) {
    return (
      <p style={{ marginTop: "1rem" }}>
        Only the cube owner can manage it. <Link href={cubePath(cubeId)}>Back to the cube</Link>.
      </p>
    );
  }

  return (
    <div style={{ maxWidth: 820, marginTop: "1rem" }}>
      <SettingsPanel cubeId={cubeId} onSaved={refreshSession} />
      <MembersPanel cubeId={cubeId} />
      <CombosPanel cubeId={cubeId} />
      <DangerZonePanel cubeId={cubeId} cubeName={view.cube.name} />
    </div>
  );
}

// --- settings + pool sync ---

function isActive(s?: CubeSyncStatus): boolean {
  return s?.status === "queued" || s?.status === "resolving" || s?.status === "downloading";
}

function SettingsPanel({ cubeId, onSaved }: { cubeId: string; onSaved: () => void }) {
  const [name, setName] = useState("");
  const [moxfieldUrl, setMoxfieldUrl] = useState("");
  const [description, setDescription] = useState("");
  const [cardList, setCardList] = useState("");
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [progress, setProgress] = useState<CubeSyncStatus | undefined>();
  const timer = useRef<ReturnType<typeof setInterval>>();

  useEffect(() => {
    apiGetOptional<CubeView>(`/cubes/${cubeId}`).then((v) => {
      if (!v) return;
      setName(v.cube.name);
      setMoxfieldUrl(v.cube.moxfield_public_id ?? "");
      setDescription(v.cube.description ?? "");
      setCardList(v.cube.card_list ?? "");
    });
    return () => clearInterval(timer.current);
  }, [cubeId]);

  function poll() {
    clearInterval(timer.current);
    const run = async () => {
      try {
        const s = await apiGetNoStore<CubeSyncStatus>(`/cubes/${cubeId}/sync-status`);
        setProgress(s);
        if (s.status === "done" || s.status === "failed" || s.status === "none") clearInterval(timer.current);
      } catch {
        /* transient; keep polling */
      }
    };
    timer.current = setInterval(run, 1500);
    void run();
  }

  async function save(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setErr(null);
    try {
      const hadList = cardList.trim() !== "";
      await apiPatch<CubeView>(`/cubes/${cubeId}`, {
        name,
        moxfield_url: moxfieldUrl,
        description,
        card_list: cardList,
      });
      if (hadList) poll();
      onSaved();
    } catch (e) {
      setErr(String(e instanceof Error ? e.message : e));
    } finally {
      setBusy(false);
    }
  }

  async function sync() {
    setErr(null);
    setProgress({ status: "queued" });
    try {
      await apiPost(`/cubes/${cubeId}/sync`);
    } catch (e) {
      setErr(String(e instanceof Error ? e.message : e));
      setProgress(undefined);
      return;
    }
    poll();
  }

  return (
    <form onSubmit={save} className="card">
      <h2 style={{ marginTop: 0 }}>Cube settings</h2>

      <label htmlFor="name">Name</label>
      <input id="name" value={name} onChange={(e) => setName(e.target.value)} required />

      <label htmlFor="mox">Moxfield URL (optional, for reference)</label>
      <input
        id="mox"
        value={moxfieldUrl}
        onChange={(e) => setMoxfieldUrl(e.target.value)}
        placeholder="https://www.moxfield.com/decks/…"
      />

      <label htmlFor="desc">Description (optional)</label>
      <textarea
        id="desc"
        value={description}
        onChange={(e) => setDescription(e.target.value)}
        rows={3}
        style={{ resize: "vertical" }}
      />

      <label htmlFor="cards">Card list</label>
      <textarea
        id="cards"
        value={cardList}
        onChange={(e) => setCardList(e.target.value)}
        rows={12}
        placeholder={"One card per line:\n1 Sol Ring\n1 Lightning Bolt\nMana Crypt"}
        style={{ resize: "vertical", fontFamily: "monospace" }}
      />
      <p className="muted" style={{ margin: "0.25rem 0 0", fontSize: "0.8rem" }}>
        The pool is built from this list. Saving re-resolves cards against Scryfall and recomputes
        analytics.
      </p>

      {err && <p style={{ color: "var(--bad)", marginTop: "0.75rem" }}>{err}</p>}

      <div style={{ display: "flex", gap: "0.75rem", marginTop: "1rem", flexWrap: "wrap" }}>
        <button className="button" disabled={busy}>
          {busy ? "Saving…" : "Save changes"}
        </button>
        {cardList.trim() !== "" && (
          <button
            type="button"
            className="button"
            onClick={sync}
            disabled={isActive(progress)}
            style={{ background: "var(--surface)", color: "var(--text)", border: "1px solid var(--border)" }}
          >
            {isActive(progress) ? "Syncing…" : "Re-sync from Scryfall"}
          </button>
        )}
      </div>
      <SyncProgress status={progress} />
    </form>
  );
}

function SyncProgress({ status }: { status?: CubeSyncStatus }) {
  if (!status || status.status === "none") return null;
  const barStyle = { marginTop: "0.5rem", fontSize: "0.85rem" } as const;

  if (status.status === "failed") {
    return <p style={{ color: "var(--bad)", ...barStyle }}>✗ Sync failed{status.error ? `: ${status.error}` : ""}</p>;
  }
  if (status.status === "done") {
    const unresolved = status.unresolved ?? [];
    return (
      <div style={barStyle}>
        <p style={{ color: "var(--good, #0a0)", margin: 0 }}>
          ✓ Synced {status.cards_total ?? 0} cards · {status.images_done ?? 0} images
          {status.images_failed ? ` · ${status.images_failed} failed` : ""}
        </p>
        {unresolved.length > 0 && (
          <p style={{ color: "var(--bad)", margin: "0.35rem 0 0" }}>
            ⚠ {unresolved.length} name{unresolved.length === 1 ? "" : "s"} not found on Scryfall and
            not in the pool — check: {unresolved.join(", ")}
          </p>
        )}
      </div>
    );
  }
  const total = status.images_total ?? 0;
  const done = status.images_done ?? 0;
  const pct = total > 0 ? Math.round((done / total) * 100) : 0;
  const label =
    status.status === "downloading"
      ? `Downloading images ${done} / ${total}`
      : status.status === "resolving"
        ? "Resolving cards…"
        : "Queued…";
  return (
    <div style={barStyle}>
      <span className="muted">{label}</span>
      {status.status === "downloading" && total > 0 && (
        <div
          style={{
            marginTop: "0.35rem",
            height: "0.5rem",
            borderRadius: "0.25rem",
            background: "var(--surface)",
            border: "1px solid var(--border)",
            overflow: "hidden",
          }}
        >
          <div style={{ width: `${pct}%`, height: "100%", background: "var(--good, #0a0)", transition: "width 0.3s ease" }} />
        </div>
      )}
    </div>
  );
}

// --- members + invites ---

function MembersPanel({ cubeId }: { cubeId: string }) {
  const [members, setMembers] = useState<PublicUser[]>([]);
  const [invites, setInvites] = useState<CubeInvite[]>([]);
  const [allUsers, setAllUsers] = useState<PublicUser[]>([]);
  const [err, setErr] = useState<string | null>(null);

  function refresh() {
    apiGetOptional<PublicUser[]>(`/cubes/${cubeId}/members`).then((m) => setMembers(m ?? []));
    apiGetOptional<CubeInvite[]>(`/cubes/${cubeId}/invites`).then((i) => setInvites(i ?? []));
  }
  useEffect(() => {
    refresh();
    apiGetOptional<PublicUser[]>("/users").then((us) => setAllUsers(us ?? []));
  }, [cubeId]);

  // The search offers everyone who isn't already a member or already invited.
  const memberIds = new Set(members.map((m) => m.id));
  const pendingIds = new Set(invites.map((i) => i.invitee_id));
  const candidates = allUsers.filter((u) => !memberIds.has(u.id) && !pendingIds.has(u.id));

  async function invite(u: PublicUser) {
    setErr(null);
    try {
      await apiPost(`/cubes/${cubeId}/invites`, { username: u.username });
      refresh();
    } catch (e) {
      setErr(String(e instanceof Error ? e.message : e));
    }
  }

  async function removeMember(u: PublicUser) {
    if (!window.confirm(`Remove ${u.display_name} from this cube?`)) return;
    try {
      await apiDelete(`/cubes/${cubeId}/members/${u.id}`);
      refresh();
    } catch (e) {
      setErr(String(e instanceof Error ? e.message : e));
    }
  }

  return (
    <div className="card" style={{ marginTop: "1rem" }}>
      <h2 style={{ marginTop: 0 }}>Members</h2>

      <label>Invite a member</label>
      <UserSearch users={candidates} onSelect={invite} placeholder="Search users to invite…" />
      {err && <p style={{ color: "var(--bad)", marginTop: "0.5rem" }}>{err}</p>}

      <div style={{ marginTop: "1rem" }}>
        {members.map((u) => (
          <MemberRow key={u.id} user={u} onRemove={() => removeMember(u)} />
        ))}
      </div>

      {invites.length > 0 && (
        <>
          <h3 style={{ margin: "1rem 0 0.25rem" }}>Pending invites</h3>
          <ul style={{ listStyle: "none", padding: 0, margin: 0 }}>
            {invites.map((i) => (
              <li key={i.id} className="muted" style={{ padding: "0.2rem 0" }}>
                {i.invitee_name} — invited, awaiting response
              </li>
            ))}
          </ul>
        </>
      )}
    </div>
  );
}

// --- combos ---

const MIN_PIECES = 2;
const MAX_PIECES = 10;

function frontFace(name: string): string {
  const i = name.indexOf("/");
  return i >= 0 ? name.slice(0, i).trim() : name;
}

function poolIndex(cards: CubeCard[]): Map<string, CubeCard> {
  const idx = new Map<string, CubeCard>();
  for (const c of cards) {
    idx.set(c.card_name.toLowerCase(), c);
    idx.set(frontFace(c.card_name).toLowerCase(), c);
  }
  return idx;
}

function CombosPanel({ cubeId }: { cubeId: string }) {
  const [pool, setPool] = useState<CubeCard[]>([]);
  const [combos, setCombos] = useState<Combo[]>([]);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [pieces, setPieces] = useState<string[]>(["", ""]);
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  function refreshCombos() {
    apiGetOptional<Combo[]>(`/cubes/${cubeId}/combos`).then((cs) => setCombos(cs ?? []));
  }
  useEffect(() => {
    apiGetOptional<CubeCard[]>(`/cubes/${cubeId}/cards`).then((cs) => setPool(cs ?? []));
    refreshCombos();
  }, [cubeId]);

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
      if (editingId) await apiPatch<Combo>(`/combos/${editingId}`, body);
      else await apiPost<Combo>(`/cubes/${cubeId}/combos`, body);
      resetForm();
      refreshCombos();
    } catch (e) {
      setErr(String(e instanceof Error ? e.message : e));
    } finally {
      setBusy(false);
    }
  }

  async function remove(combo: Combo) {
    if (!window.confirm(`Delete combo "${combo.name}"? Decks will stop reporting it.`)) return;
    try {
      await apiDelete(`/combos/${combo.id}`);
      if (editingId === combo.id) resetForm();
      refreshCombos();
    } catch (e) {
      setErr(String(e instanceof Error ? e.message : e));
    }
  }

  return (
    <div className="card" style={{ marginTop: "1rem" }}>
      <h2 style={{ marginTop: 0 }}>Combos</h2>
      <p className="muted" style={{ marginTop: "-0.25rem", fontSize: "0.85rem" }}>
        Name a set of cards that play together; any deck whose mainboard holds all of them lists the
        combo on its page.
      </p>

      <datalist id="pool-cards">
        {pool.map((c) => (
          <option key={c.card_id} value={c.card_name} />
        ))}
      </datalist>

      <form onSubmit={submit} style={{ marginTop: "0.75rem" }}>
        <label htmlFor="cname">{editingId ? "Edit combo" : "New combo"}</label>
        <input id="cname" value={name} onChange={(e) => setName(e.target.value)} placeholder="Thoracle" required />

        <label htmlFor="cdesc">Description (optional)</label>
        <input
          id="cdesc"
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
                style={{ background: "var(--surface)", color: "var(--text)", border: "1px solid var(--border)", flexShrink: 0 }}
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
          style={{ marginTop: "0.5rem", background: "var(--surface)", color: "var(--text)", border: "1px solid var(--border)" }}
        >
          + Add card
        </button>

        {err && <p style={{ color: "var(--bad)", marginTop: "0.75rem" }}>{err}</p>}

        <div style={{ display: "flex", gap: "0.75rem", marginTop: "1rem" }}>
          <button className="button" disabled={busy}>
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

      <div style={{ display: "flex", flexDirection: "column", gap: "0.5rem", marginTop: "1rem" }}>
        {combos.map((combo) => (
          <div key={combo.id} style={{ borderTop: "1px solid var(--grid)", paddingTop: "0.5rem" }}>
            <strong>{combo.name}</strong>
            <span className="muted"> — {combo.cards.map((c) => c.card_name).join(" + ")}</span>
            <div style={{ display: "flex", gap: "0.75rem", marginTop: "0.25rem" }}>
              <button type="button" className="ghost-button" onClick={() => startEdit(combo)}>
                Edit
              </button>
              <button type="button" className="ghost-button" onClick={() => remove(combo)}>
                Delete
              </button>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}

// --- delete ---

// Deleting a cube cascades to its pool, decks, combos, members, and invites, so it asks
// the owner to type the cube's name — the same friction GitHub uses for an irreversible,
// far-reaching delete.
function DangerZonePanel({ cubeId, cubeName }: { cubeId: string; cubeName: string }) {
  const router = useRouter();
  const [confirm, setConfirm] = useState("");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const armed = confirm.trim() === cubeName;

  async function del() {
    if (!armed) return;
    setBusy(true);
    setErr(null);
    try {
      await apiDelete(`/cubes/${cubeId}`);
      router.push("/");
    } catch (e) {
      setErr(String(e instanceof Error ? e.message : e));
      setBusy(false);
    }
  }

  return (
    <div className="card" style={{ marginTop: "1rem", borderColor: "var(--bad, #b00)" }}>
      <h2 style={{ marginTop: 0 }}>Danger zone</h2>
      <p className="muted" style={{ marginTop: 0, fontSize: "0.85rem" }}>
        Deleting removes the cube, its card pool, every deck in it, its combos, and its
        member list — permanently. This cannot be undone.
      </p>
      <label htmlFor="confirm-name">
        Type <strong>{cubeName}</strong> to confirm
      </label>
      <input
        id="confirm-name"
        value={confirm}
        onChange={(e) => setConfirm(e.target.value)}
        autoComplete="off"
      />
      {err && <p style={{ color: "var(--bad)", marginTop: "0.5rem" }}>{err}</p>}
      <button
        type="button"
        className="button"
        onClick={del}
        disabled={!armed || busy}
        style={{
          marginTop: "0.75rem",
          background: armed ? "var(--bad, #b00)" : "var(--surface)",
          color: armed ? "#fff" : "var(--muted)",
          border: "1px solid var(--border)",
        }}
      >
        {busy ? "Deleting…" : "Delete cube"}
      </button>
    </div>
  );
}
