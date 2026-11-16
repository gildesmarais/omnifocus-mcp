(function (args) {
  // args.items: {kind, id}. Reversible. A tag may be dropped only without remaining tasks.
  return eachItem(args.items, p => {
    const o = find(p.kind, p.id);
    if (p.kind === "task") o.drop(true);
    else if (p.kind === "project") o.status = Project.Status.Dropped;
    else if (p.kind === "folder") o.status = Folder.Status.Dropped;
    else {
      // The tag and every descendant tag; tasks under child tags also block the drop.
      const ids = new Set([o.id.primaryKey]);
      flattenedTags.forEach(g => { if (inside(o, g)) ids.add(g.id.primaryKey); });
      const n = new Set(flattenedTags.filter(g => ids.has(g.id.primaryKey)).flatMap(g => g.remainingTasks.map(t => t.id.primaryKey))).size;
      if (n > 0) throw new Error("tag still has " + n + " remaining task(s); untag or complete them first");
      o.status = Tag.Status.Dropped;
    }
    return shapeStruct(p.kind, o);
  });
})
