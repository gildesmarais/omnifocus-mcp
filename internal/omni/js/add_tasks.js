(function (args) {
  // args.items: validated additions. One result per item; a failing item never aborts the batch.
  return args.items.map(a => {
    try {
      let pos = inbox.ending;
      if (a.project) pos = need(Project.byIdentifier(a.project), "project").ending;
      else if (a.parent) pos = findTask(a.parent, "parent task").ending;
      const write = fieldWriter(a, a.tags, []);
      const t = new Task(a.name, pos);
      write(t);
      return { id: t.id.primaryKey, ok: true, task: shapeTask(t) };
    } catch (e) {
      return { ok: false, error: String(e.message || e) };
    }
  });
})
