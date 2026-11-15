(function (args) {
  // Serves list_tags and get_tag (args.id).
  const ref = o => o ? { id: o.id.primaryKey, name: o.name } : null;
  const statusName = s => (/: (\w+)\]/.exec(String(s)) || [])[1];
  const shape = t => {
    const o = { id: t.id.primaryKey, name: t.name, status: statusName(t.status), remainingTasks: t.remainingTasks.length };
    const par = ref(t.parent); if (par) o.parent = par;
    return o;
  };
  if (args.id) {
    const t = Tag.byIdentifier(args.id);
    return t ? [shape(t)] : [];
  }
  return flattenedTags.map(shape);
})
