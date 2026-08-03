"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import {
  apiGetOptional,
  apiPost,
  type CubeInvite,
  type CubeView,
  type DecklistDetail,
} from "@/lib/api";
import { cubePath } from "@/lib/cube";
import { useSession } from "@/components/SessionProvider";
import { CardMarqueeBackground } from "@/components/CardMarqueeBackground";

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
  const [cubes, setCubes] = useState<CubeView[] | null>(null);
  const [invites, setInvites] = useState<CubeInvite[]>([]);
  const [newName, setNewName] = useState("");
  const [creating, setCreating] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  function refresh() {
    apiGetOptional<CubeView[]>("/cubes", 0).then((cs) => setCubes(cs ?? []));
    apiGetOptional<CubeInvite[]>("/me/invites", 0).then((i) => setInvites(i ?? []));
  }
  useEffect(refresh, []);

  async function respond(invite: CubeInvite, accept: boolean) {
    try {
      await apiPost(`/invites/${invite.id}/${accept ? "accept" : "decline"}`);
      refresh();
    } catch (e) {
      setErr(String(e instanceof Error ? e.message : e));
    }
  }

  async function createCube(e: React.FormEvent) {
    e.preventDefault();
    setErr(null);
    setCreating(true);
    try {
      // The new cube starts empty; drop the owner on its Manage tab to paste a list.
      const view = await apiPost<CubeView>("/cubes", { name: newName.trim() });
      router.push(cubePath(view.cube.id, "/manage"));
    } catch (e) {
      setErr(String(e instanceof Error ? e.message : e));
      setCreating(false);
    }
  }

  return (
    <main className="container">
      <h1>Your cubes</h1>

      {invites.length > 0 && (
        <div className="card" style={{ marginTop: "1rem" }}>
          <h2 style={{ marginTop: 0 }}>Invites</h2>
          {invites.map((i) => (
            <div
              key={i.id}
              style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: "0.75rem", padding: "0.35rem 0", flexWrap: "wrap" }}
            >
              <span>
                <strong>{i.cube_name}</strong>
                {i.invited_by && <span className="muted"> · invited by {i.invited_by}</span>}
              </span>
              <span style={{ display: "flex", gap: "0.5rem" }}>
                <button type="button" className="button" onClick={() => respond(i, true)}>
                  Accept
                </button>
                <button type="button" className="ghost-button" onClick={() => respond(i, false)}>
                  Decline
                </button>
              </span>
            </div>
          ))}
        </div>
      )}

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
