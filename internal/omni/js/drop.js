(function (args) {
  // args.items: {kind, id}. Reversible. A tag may be dropped only without remaining tasks.
  return eachItem(args.items, p => {
    const o = find(p.kind, p.id);
    if (p.kind === "task") o.drop(true);
    else if (p.kind === "project") o.status = Project.Status.Dropped;
    else if (p.kind === "folder") o.status = Folder.Status.Dropped;
    else {
      const n = o.remainingTasks.length;
      if (n > 0) throw new Error("tag still has " + n + " remaining task(s); untag or complete them first");
      o.status = Tag.Status.Dropped;
    }
    return shapeStruct(p.kind, o);
  });
})
