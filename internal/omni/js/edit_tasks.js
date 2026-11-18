(function (args) {
  // args.items: validated patches. Lookups run before any mutation so a bad
  // target leaves the task untouched; one result per item, failures never abort the batch.
  return args.items.map(p => {
    try {
      const t = findTask(p.id);
      let pos = null;
      if (p.move) {
        if (p.move.inbox) pos = inbox.ending;
        else if (p.move.project) pos = need(Project.byIdentifier(p.move.project), "project").ending;
        else {
          const par = findTask(p.move.parent, "parent task");
          if (inside(t, par)) throw new Error("cannot move task into itself or its subtask");
          pos = par.ending;
        }
      }
      const write = fieldWriter(p, p.tagsAdd, p.tagsRemove);
      if (pos) moveTasks([t], pos);
      if (p.restore) t.active = true;
      if (has(p, "name")) t.name = p.name;
      write(t);
      if (p.complete) t.markComplete();
      if (p.uncomplete) t.markIncomplete();
      return { id: p.id, ok: true, task: shapeTask(t) };
    } catch (e) {
      return { id: p.id, ok: false, error: String(e.message || e) };
    }
  });
})
