(function (args) {
  // args.items: validated additions. One result per item; a failing item never aborts the batch.
  const need = (o, what) => { if (!o) throw new Error(what + " not found"); return o; };
  return args.items.map(a => {
    try {
      let pos = inbox.ending;
      if (a.project) pos = need(Project.byIdentifier(a.project), "project").ending;
      else if (a.parent) pos = findTask(a.parent, "parent task").ending;
      const tags = (a.tags || []).map(id => need(Tag.byIdentifier(id), "tag " + id));
      const t = new Task(a.name, pos);
      if (a.note) t.note = a.note;
      if (a.flagged) t.flagged = true;
      if (a.defer) t.deferDate = parseDate(a.defer);
      if (a.due) t.dueDate = parseDate(a.due);
      if (a.planned) t.plannedDate = parseDate(a.planned);
      tags.forEach(g => t.addTag(g));
      return { id: t.id.primaryKey, ok: true, task: shapeTask(t) };
    } catch (e) {
      return { ok: false, error: String(e.message || e) };
    }
  });
})
