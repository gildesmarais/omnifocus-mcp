(function (args) {
  // args.items: validated patches. Lookups run before any mutation so a bad
  // target leaves the task untouched; one result per item, failures never abort the batch.
  const need = (o, what) => { if (!o) throw new Error(what + " not found"); return o; };
  const has = (p, k) => Object.prototype.hasOwnProperty.call(p, k);
  const date = (p, k, set) => { if (has(p, k)) set(p[k] === null ? null : parseDate(p[k])); };
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
      const tagsOf = ids => (ids || []).map(id => need(Tag.byIdentifier(id), "tag " + id));
      const add = tagsOf(p.tagsAdd), rem = tagsOf(p.tagsRemove);
      if (pos) moveTasks([t], pos);
      if (has(p, "name")) t.name = p.name;
      if (has(p, "note")) t.note = p.note === null ? "" : p.note;
      if (has(p, "flagged")) t.flagged = p.flagged;
      date(p, "defer", d => { t.deferDate = d; });
      date(p, "due", d => { t.dueDate = d; });
      date(p, "planned", d => { t.plannedDate = d; });
      add.forEach(g => t.addTag(g));
      rem.forEach(g => t.removeTag(g));
      if (p.complete) t.markComplete();
      if (p.uncomplete) t.markIncomplete();
      return { id: p.id, ok: true, task: shapeTask(t) };
    } catch (e) {
      return { id: p.id, ok: false, error: String(e.message || e) };
    }
  });
})
