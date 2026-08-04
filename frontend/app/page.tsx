"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { apiGetOptional, apiPost, type CubeView, type PublicUser } from "@/lib/api";
import { cubePath } from "@/lib/cube";
import { useSession } from "@/components/SessionProvider";
import { CardMarqueeBackground } from "@/components/CardMarqueeBackground";
import { InviteBanner } from "@/components/InviteBanner";
import { UserSearch, MemberRow } from "@/components/UserSearch";

// Anonymous visitors get the marketing landing; a signed-in user gets their dashboard —
// the cubes they belong to and any invites waiting. This is the root, replacing the old
// redirect to a global cube list.
export default function Home() {
  const { me } = useSession();

  if (me === undefined) return <main className="container" aria-hidden />;
  if (me) return <Dashboard />;

  return (
    <main className="landing">
      {/* One marquee for the whole page, not one per section: it is viewport-pinned,
          so every panel below scrolls over the same continuous drift. */}
      <CardMarqueeBackground />
      <section className="landing-hero">
        <div className="landing-hero-inner">
          <h1 className="landing-title">
            🎴 Meta Tracker
          </h1>
          <p className="landing-tagline">
            A metagame dashboard for your local Magic: The Gathering cube.
          </p>
          <div className="landing-cta">
            <Link href="/login" className="button">
              Sign in
            </Link>
            <p className="muted" style={{ marginTop: "0.75rem", fontSize: "0.85rem" }}>
              No account? Ask an admin to create one for you.
            </p>
          </div>
          <div className="landing-hero-hint" aria-hidden>
            <span>Scroll</span>
            <span className="landing-hero-hint-arrow">↓</span>
          </div>
        </div>
      </section>

      <LandingFeature
        icon="🃏"
        title="Build and Manage Decks and Cubes"
        body="Paste a cube list and every card resolves against Scryfall. Players upload the decks they built from the pool, and the site tracks each printing, casting cost, and color."
        variant="a"
      />
      <LandingFeature
        icon="📊"
        title="Track Your Metagame"
        body="Aggregate snapshots surface color share, card popularity, card co-occurrence, and deck-level metrics — the shape of your playgroup's meta, recomputed as new decks land."
        variant="b"
      />
      <LandingFeature
        icon="👥"
        title="Share With Your Playgroup"
        body="One deployment for one group. Everyone signs in, everyone's decks live in the same table, and the stats reflect what the room is actually playing."
        variant="c"
      />
    </main>
  );
}

// The signed-in home. Lists the cubes you belong to, any pending invites, and a way to
// spin up a new cube (you own what you create).
function Dashboard() {
  const router = useRouter();
  const { me } = useSession();
  const [cubes, setCubes] = useState<CubeView[] | null>(null);
  // Everyone but the caller, for the "invite on create" picker. Usernames aren't secret,
  // and the playgroup is small, so listing them is fine.
  const [others, setOthers] = useState<PublicUser[]>([]);
  const [invitees, setInvitees] = useState<string[]>([]);
  const [newName, setNewName] = useState("");
  const [creating, setCreating] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  function refreshCubes() {
    apiGetOptional<CubeView[]>("/cubes", 0).then((cs) => setCubes(cs ?? []));
  }
  useEffect(() => {
    refreshCubes();
    apiGetOptional<PublicUser[]>("/users", 0).then((us) =>
      setOthers((us ?? []).filter((u) => u.id !== me?.id)),
    );
  }, [me?.id]);

  function removeInvitee(id: string) {
    setInvitees((prev) => prev.filter((x) => x !== id));
  }

  // Candidates the search offers (everyone not already picked) and the picked users
  // themselves, resolved back to full records for the list below.
  const candidates = others.filter((u) => !invitees.includes(u.id));
  const selectedUsers = invitees
    .map((id) => others.find((u) => u.id === id))
    .filter((u): u is PublicUser => u != null);

  async function createCube(e: React.FormEvent) {
    e.preventDefault();
    setErr(null);
    setCreating(true);
    try {
      // The cube starts empty; its Manage page is where you paste a list. Fire off the
      // chosen invites, then land on Manage so the owner can finish setup.
      const view = await apiPost<CubeView>("/cubes", { name: newName.trim() });
      const chosen = others.filter((u) => invitees.includes(u.id));
      await Promise.all(
        chosen.map((u) => apiPost(`/cubes/${view.cube.id}/invites`, { username: u.username })),
      );
      router.push(cubePath(view.cube.id, "/manage"));
    } catch (e) {
      setErr(String(e instanceof Error ? e.message : e));
      setCreating(false);
    }
  }

  return (
    <main className="container">
      <h1>Your cubes</h1>

      <InviteBanner />

      {cubes === null ? (
        <p className="muted" style={{ marginTop: "1rem" }}>
          Loading…
        </p>
      ) : cubes.length === 0 ? (
        <p className="muted" style={{ marginTop: "1rem" }}>
          You&apos;re not in any cubes yet. Create one below, or ask a cube owner to invite you.
        </p>
      ) : (
        <div
          className="grid"
          style={{ marginTop: "1rem", gridTemplateColumns: "repeat(auto-fill, minmax(220px, 1fr))", gap: "0.75rem" }}
        >
          {cubes.map((cv) => (
            <Link key={cv.cube.id} href={cubePath(cv.cube.id)} className="card" style={{ textDecoration: "none" }}>
              <strong style={{ fontSize: "1.05rem" }}>{cv.cube.name}</strong>
              <p className="muted" style={{ margin: "0.25rem 0 0", fontSize: "0.85rem" }}>
                {cv.card_count} cards
              </p>
            </Link>
          ))}
        </div>
      )}

      <form onSubmit={createCube} className="card" style={{ marginTop: "1.5rem", maxWidth: 460 }}>
        <h2 style={{ marginTop: 0 }}>New cube</h2>
        <label htmlFor="cubename">Name</label>
        <input id="cubename" value={newName} onChange={(e) => setNewName(e.target.value)} required />

        {others.length > 0 && (
          <>
            <label style={{ marginTop: "0.75rem" }}>Invite members (optional)</label>
            <UserSearch
              users={candidates}
              onSelect={(u) => setInvitees((prev) => [...prev, u.id])}
              placeholder="Search users to invite…"
            />
            {selectedUsers.length > 0 && (
              <div style={{ marginTop: "0.5rem" }}>
                {selectedUsers.map((u) => (
                  <MemberRow key={u.id} user={u} onRemove={() => removeInvitee(u.id)} />
                ))}
              </div>
            )}
            <p className="muted" style={{ margin: "0.35rem 0 0", fontSize: "0.8rem" }}>
              They&apos;ll get an invite to accept. You can add or remove members later from Manage cube.
            </p>
          </>
        )}

        {err && <p style={{ color: "var(--bad)", marginTop: "0.5rem" }}>{err}</p>}
        <button className="button" style={{ marginTop: "0.75rem" }} disabled={creating || newName.trim() === ""}>
          {creating ? "Creating…" : "Create cube"}
        </button>
      </form>
    </main>
  );
}

interface LandingFeatureProps {
  icon: string;
  title: string;
  body: string;
  variant: "a" | "b" | "c";
}

// A parallax section, in three planes: the page-wide marquee behind everything
// (pinned, so it never moves), this section's colour wash (scrolls at page
// speed), and the caption (sticky, so it holds still while its wash travels
// under it and then releases when the next section arrives).
function LandingFeature({ icon, title, body, variant }: LandingFeatureProps) {
  return (
    <section className={`landing-feature landing-feature-${variant}`}>
      <div className="landing-feature-bg" aria-hidden />
      <div className="landing-feature-inner">
        <div className="landing-feature-icon" aria-hidden>
          {icon}
        </div>
        <h2 className="landing-feature-title">{title}</h2>
        <p className="landing-feature-body">{body}</p>
      </div>
    </section>
  );
}
