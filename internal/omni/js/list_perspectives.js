(function (args) {
  // Built-in perspectives have no identifier; custom ones do.
  const builtin = Perspective.BuiltIn.all.map(p => ({ id: null, name: p.name, builtin: true }));
  const custom = Perspective.Custom.all.map(p => ({ id: p.identifier, name: p.name, builtin: false }));
  return builtin.concat(custom);
})
