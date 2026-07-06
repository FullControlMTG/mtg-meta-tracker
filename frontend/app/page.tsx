export default function Home() {
  return (
    <main style={{ fontFamily: "system-ui", padding: "2rem", maxWidth: 720 }}>
      <h1>MTG Meta Tracker</h1>
      <p>Meta analysis for your local cube playgroup.</p>
      <ul>
        <li>/analytics — interactive meta dashboard</li>
        <li>/decklists — browse decks (static, revalidated on update)</li>
        <li>/users/[name] — player pages</li>
        <li>/decks/new — upload a deck (live color inference)</li>
      </ul>
      <p style={{ color: "#888" }}>Phase 0 scaffold — see docs/ROADMAP.md.</p>
    </main>
  );
}
