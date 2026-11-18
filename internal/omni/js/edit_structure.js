(function (args) {
  // args.items: validated patches. Every lookup and guard runs before the first
  // mutation so a bad target leaves the object untouched.
  return eachItem(args.items, p => {
    const o = find(p.kind, p.id);
    const tag = p.kind === "tag", nested = p.kind !== "project";
    const write = p.kind === "project" ? fieldWriter(p, p.tags && p.tags.add, p.tags && p.tags.remove) : null;
    let pos = null;
    if (p.move) {
      if (p.move.parent === null) pos = tag ? tags.ending : library.ending;
      else {
        const par = find(tag ? "tag" : "folder", p.move.parent);
        if (nested && inside(o, par)) throw new Error("cannot move " + p.kind + " into itself");
        pos = par.ending;
      }
    } else if (p.reorder) {
      const sib = find(p.kind, p.reorder.before || p.reorder.after);
      if (nested && inside(o, sib)) throw new Error("cannot reorder " + p.kind + " relative to its own descendant");
      pos = p.reorder.before ? sib.before : sib.after;
    }
    if (pos) { if (tag) moveTags([o], pos); else moveSections([o], pos); }
    if (has(p, "name")) o.name = p.name;
    if (p.status) o.status = byKind[p.kind].Status[p.status];
    if (write) write(o);
    if (p.reviewInterval) { // no constructor: mutate the existing object and assign it back
      const r = o.reviewInterval;
      r.steps = p.reviewInterval.steps;
      r.unit = p.reviewInterval.unit;
      o.reviewInterval = r;
    }
    if (p.nextReviewDate) o.nextReviewDate = parseDate(p.nextReviewDate);
    if (p.reviewed) o.lastReviewDate = new Date(); // OmniFocus recomputes nextReviewDate from the interval
    return shapeStruct(p.kind, o);
  });
})
