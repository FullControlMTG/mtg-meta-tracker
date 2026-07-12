// Placement for the floating overlays — the card art on a stats row, the radar's
// data-point tooltip.
//
// They are positioned with `position: fixed`, i.e. in viewport coordinates, and that is
// load-bearing: the stats tables live inside `<div style={{overflowX: "auto"}}>`, and a
// scroll container clips absolutely-positioned descendants but not fixed ones. (The
// .info-bubble solves the same problem the other way, by hanging inside the container.)
// The catch: a transform/filter/will-change on any ancestor would make it the containing
// block for `fixed` and bring the clipping back — if that ever lands on .card/.container,
// these have to move to a portal on document.body.

export interface Placed {
  left: number;
  top: number;
}

const clamp = (v: number, lo: number, hi: number) => Math.min(Math.max(v, lo), hi);

// Prefer the space right of the anchor; flip to its left when the box would run off the
// edge. Vertically centered, then clamped into the viewport.
export function placeFloating(x: number, y: number, w: number, h: number, gap = 16): Placed {
  const vw = window.innerWidth;
  const vh = window.innerHeight;
  const right = x + gap;
  const left = right + w <= vw - gap ? right : x - gap - w;
  return {
    left: clamp(left, gap, Math.max(gap, vw - w - gap)),
    top: clamp(y - h / 2, gap, Math.max(gap, vh - h - gap)),
  };
}

// A hover preview is a pointer affordance: a touch device has no hover, and a 240px card
// over a 375px viewport is just an obstruction. Read it from an effect — it touches
// window, so it must not run while rendering on the server.
export const canHover = () =>
  typeof window !== "undefined" &&
  window.matchMedia("(hover: hover) and (pointer: fine)").matches;
