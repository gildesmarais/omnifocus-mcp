(function (args) {
  // Serves list_folders and get_folder (args.id).
  const ref = o => o ? { id: o.id.primaryKey, name: o.name } : null;
  const statusName = s => (/: (\w+)\]/.exec(String(s)) || [])[1];
  const shape = f => {
    const o = { id: f.id.primaryKey, name: f.name, status: statusName(f.status) };
    const par = ref(f.parent); if (par) o.parent = par;
    return o;
  };
  if (args.id) {
    const f = Folder.byIdentifier(args.id);
    return f ? [shape(f)] : [];
  }
  return flattenedFolders.map(shape);
})
