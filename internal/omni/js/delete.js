(function (args) {
  // args.items: {kind, id}. Irreversible. Tasks and projects only without remaining
  // (incomplete) tasks, else drop them; tags and folders only when already dropped and empty.
  return eachItem(args.items, p => {
    const o = find(p.kind, p.id);
    if (p.kind === "project" || p.kind === "task") {
      const n = remaining(p.kind === "project" ? o.flattenedTasks : o.flattenedChildren).length;
      if (n > 0) throw new Error(p.kind + " still has " + n + " remaining " + (p.kind === "project" ? "task" : "subtask") + "(s); drop it instead, or complete or drop them first");
    }
    if (p.kind === "tag" || p.kind === "folder") {
      if (statusName(o.status) !== "Dropped") throw new Error(p.kind + " is not dropped; drop it first");
      const kids = p.kind === "tag" ? o.children.length + o.remainingTasks.length : o.sections.length;
      if (kids > 0) throw new Error(p.kind + " is not empty (" + kids + " child object(s)); delete or move them first");
    }
    deleteObject(o);
    return { id: p.id, kind: p.kind, ok: true };
  });
})
