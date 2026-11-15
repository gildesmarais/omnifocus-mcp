(function (args) {
  // args.items: validated additions of projects, tags and folders.
  return eachItem(args.items, a => {
    const tag = a.kind === "tag";
    const pos = a.parent ? find(tag ? "tag" : "folder", a.parent).ending : (tag ? tags.ending : library.ending);
    const o = tag ? new Tag(a.name, pos) : a.kind === "project" ? new Project(a.name, pos) : new Folder(a.name, pos);
    if (a.status) o.status = (tag ? Tag.Status : Project.Status)[a.status];
    if (a.sequential) o.sequential = true;
    return shapeStruct(a.kind, o);
  });
})
