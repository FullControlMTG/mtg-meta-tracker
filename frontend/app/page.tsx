"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { useSession } from "@/components/SessionProvider";
import { CardMarqueeBackground } from "@/components/CardMarqueeBackground";

// Anonymous visitors get the marketing landing; a signed-in user goes to /cubes,
// the hub they picked a cube from before anything cube-scoped rendered.
export default function Home() {
  const router = useRouter();
  const { me } = useSession();

  useEffect(() => {
    if (me) router.replace("/cubes");
  }, [me, router]);

  // While the session is loading, or an authenticated user is about to be
  // redirected, render nothing rather than a landing that will flash and vanish.
  if (me === undefined || me) {
    return <main className="container" aria-hidden />;
  }

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
