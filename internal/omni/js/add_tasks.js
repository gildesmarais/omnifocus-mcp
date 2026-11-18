(function (args) {
  // args.items: validated additions. One result per item; a failing item never aborts the batch.
  return args.items.map(a => {
    try {
      let pos = inbox.ending;
      if (a.project) pos = need(Project.byIdentifier(a.project), "project").ending;
      else if (a.parent) pos = findTask(a.parent, "parent task").ending;
      const tags = tagsOf(a.tags);
      const rule = a.repetition ? makeRule(a.repetition) : null;
      const t = new Task(a.name, pos);
      if (a.note) t.note = a.note;
      if (a.flagged) t.flagged = true;
      if (a.defer) t.deferDate = parseDate(a.defer);
      if (a.due) t.dueDate = parseDate(a.due);
      if (a.planned) t.plannedDate = parseDate(a.planned);
      if (a.estimatedMinutes != null) t.estimatedMinutes = a.estimatedMinutes;
      if (rule) t.repetitionRule = rule;
      if (a.sequential) t.sequential = true;
      if (a.completedByChildren) t.completedByChildren = true;
      tags.forEach(g => t.addTag(g));
      return { id: t.id.primaryKey, ok: true, task: shapeTask(t) };
    } catch (e) {
      return { ok: false, error: String(e.message || e) };
    }
  });
})
