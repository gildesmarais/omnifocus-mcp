(function (args) {
  // Serves list_projects and get_project (args.id).

  function shape(p) {
    const ts = statusName(p.taskStatus);
    const o = {
      id: p.id.primaryKey, name: p.name, status: statusName(p.status), flagged: p.flagged,
      sequential: p.sequential, available: OK.has(ts),
      blocked: ts === "Blocked", tags: p.tags.map(ref)
    };
    if (p.note) o.note = p.note;
    const f = ref(p.parentFolder); if (f) o.folder = f;
    return withDates(o, p);
  }

  if (args.id) {
    const p = Project.byIdentifier(args.id);
    return p ? [shape(p)] : [];
  }
  const search = args.search ? args.search.toLowerCase() : null;
  const inFolder = p => {
    for (let f = p.parentFolder; f; f = f.parent) if (match(f, args.folder)) return true;
    return false;
  };
  const limit = args.limit == null ? Infinity : args.limit;
  const out = [];
  for (const p of flattenedProjects) {
    if (out.length >= limit) break;
    if (args.status && statusName(p.status) !== args.status) continue;
    if (args.folder && !inFolder(p)) continue;
    if (search && !(p.name.toLowerCase().includes(search) || p.note.toLowerCase().includes(search))) continue;
    out.push(shape(p));
  }
  return out;
})
