(function (args) {
  // Serves list_tasks and get_task (args.id). Filters run here so only matches cross osascript.
  const parseDate = s => (s ? new Date(s.length === 10 ? s + "T00:00:00" : s) : null);
  const OK = new Set(["Available", "DueSoon", "Next", "Overdue"]);

  const dueBefore = parseDate(args.due_before), deferBefore = parseDate(args.defer_before);
  const search = args.search ? args.search.toLowerCase() : null;

  function shape(t) {
    const st = statusName(t.taskStatus);
    const o = {
      id: t.id.primaryKey, name: t.name, status: st, flagged: t.flagged,
      completed: t.completed, dropped: t.dropped, available: OK.has(st), blocked: st === "Blocked",
      inbox: t.inInbox, tags: t.tags.map(ref)
    };
    if (t.note) o.note = t.note;
    const p = ref(t.containingProject); if (p) o.project = p;
    const par = ref(t.parent); if (par) o.parent = par;
    const df = iso(t.deferDate), du = iso(t.dueDate), pl = iso(t.plannedDate);
    if (df) o.defer = df;
    if (du) o.due = du;
    if (pl) o.planned = pl;
    const dn = iso(t.completionDate); if (dn) o.completedAt = dn;
    return o;
  }

  if (args.id) {
    const t = Task.byIdentifier(args.id);
    return t ? [shape(t)] : [];
  }

  let pool = flattenedTasks;
  if (args.inbox === true) pool = inbox;
  else if (args.project) {
    const ps = flattenedProjects.filter(p => match(p, args.project));
    if (ps.length === 0) return [];
    pool = ps.flatMap(p => p.flattenedTasks);
  }
  const limit = args.limit == null ? Infinity : args.limit;
  const out = [];
  for (const t of pool) {
    if (out.length >= limit) break;
    if (args.inbox === false && t.inInbox) continue;
    if (args.flagged != null && t.flagged !== args.flagged) continue;
    if (args.completed != null && t.completed !== args.completed) continue;
    if (dueBefore && !(t.dueDate && t.dueDate < dueBefore)) continue;
    if (deferBefore && !(t.deferDate && t.deferDate < deferBefore)) continue;
    if (args.available != null && OK.has(statusName(t.taskStatus)) !== args.available) continue;
    if (args.tag && !t.tags.some(g => match(g, args.tag))) continue;
    if (search && !(t.name.toLowerCase().includes(search) || t.note.toLowerCase().includes(search))) continue;
    out.push(shape(t));
  }
  return out;
})
