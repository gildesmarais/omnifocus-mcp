(function (args) {
  // args.items: validated patches. Every lookup and guard runs before the first
  // mutation so a bad target leaves the object untouched.
  const has = (p, k) => Object.prototype.hasOwnProperty.call(p, k);
  const inside = (o, x) => { for (let c = x; c; c = c.parent) if (c.id.primaryKey === o.id.primaryKey) return true; return false; };
  return eachItem(args.items, p => {
    const o = find(p.kind, p.id);
    const tag = p.kind === "tag", nested = p.kind !== "project";
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
    if (p.status) o.status = (tag ? Tag.Status : Project.Status)[p.status];
    if (has(p, "sequential")) o.sequential = p.sequential;
    return shapeStruct(p.kind, o);
  });
})
