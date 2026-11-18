(function (args) {
  // Serves list_tasks and get_task (args.id). Filters run here so only matches cross osascript.
  const search = args.search ? args.search.toLowerCase() : null;

  if (args.id) {
    const t = Task.byIdentifier(args.id);
    if (!t) return [];
    if (isProjectRoot(t)) throw new Error("task " + args.id + " is a project, use kind project");
    return [shapeTask(t)];
  }

  let pool = flattenedTasks;
  if (args.inbox === true) pool = flattenedTasks.filter(t => t.inInbox); // `inbox` alone lists top-level tasks only
  else if (args.project) {
    const ps = flattenedProjects.filter(p => match(p, args.project));
    if (ps.length === 0) return [];
    pool = ps.flatMap(p => p.flattenedTasks);
  }
  const limit = args.limit == null ? Infinity : args.limit;
  // [test, date getter] per date field with a bound set.
  const ranges = [
    ["due", t => t.dueDate], ["defer", t => t.deferDate], ["planned", t => t.plannedDate],
    ["completed", t => t.completionDate], ["modified", t => t.modified], ["added", t => t.added]
  ].map(([k, get]) => [dateBounds(args, k), get]).filter(r => r[0]);
  const out = [];
  for (const t of pool) {
    if (out.length >= limit) break;
    if (args.inbox === false && t.inInbox) continue;
    if (args.flagged != null && t.flagged !== args.flagged) continue;
    if (args.completed != null && t.completed !== args.completed) continue;
    if (args.untagged != null && (t.tags.length === 0) !== args.untagged) continue;
    if (args.repeating != null && !!t.repetitionRule !== args.repeating) continue;
    const st = statusName(t.taskStatus);
    if (args.dropped != null && (st === "Dropped") !== args.dropped) continue;
    if (args.available != null && OK.has(st) !== args.available) continue;
    if (!ranges.every(([test, get]) => test(get(t)))) continue;
    if (args.tag && !t.tags.some(g => match(g, args.tag))) continue;
    if (search && !(t.name.toLowerCase().includes(search) || t.note.toLowerCase().includes(search))) continue;
    out.push(shapeTask(t));
  }
  return out;
})
