// Prepended by the bridge to every script. Shared helpers only; no side effects.
const pad = n => (n < 10 ? "0" : "") + n;
const iso = d => d ? d.getFullYear() + "-" + pad(d.getMonth() + 1) + "-" + pad(d.getDate()) + "T" + pad(d.getHours()) + ":" + pad(d.getMinutes()) + ":" + pad(d.getSeconds()) : null;
const ref = o => o ? { id: o.id.primaryKey, name: o.name } : null;
const statusName = s => (/: (\w+)\]/.exec(String(s)) || [])[1];
const match = (o, q) => o.id.primaryKey === q || o.name === q;
const parseDate = s => (s ? new Date(s.length === 10 ? s + "T00:00:00" : s) : null);
const OK = new Set(["Available", "DueSoon", "Next", "Overdue"]);
// A task as the Task record in read.go.
const shapeTask = t => {
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
};
// Structure helpers (add/edit/drop/delete). byKind maps a kind to its class; tags nest under tags, the rest under library.
const byKind = { task: Task, project: Project, tag: Tag, folder: Folder };
const find = (kind, id) => { const o = byKind[kind].byIdentifier(id); if (!o) throw new Error(kind + " not found"); return o; };
// A project, tag, folder or task as StructResult in write_structure.go.
const shapeStruct = (kind, o) => {
  const r = { id: o.id.primaryKey, kind: kind, ok: true, name: o.name };
  r.status = kind === "task" ? statusName(o.taskStatus) : statusName(o.status);
  const par = ref(kind === "project" ? o.parentFolder : o.parent);
  if (par) r.parent = par;
  return r;
};
// Runs fn per item; one result per item, failures never abort the batch.
const eachItem = (items, fn) => items.map(p => {
  try { return fn(p); } catch (e) { return { id: p.id, ok: false, error: String(e.message || e) }; }
});
