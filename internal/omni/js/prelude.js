// Prepended by the bridge to every script. Shared helpers only; no side effects.
const pad = n => (n < 10 ? "0" : "") + n;
const iso = d => d ? d.getFullYear() + "-" + pad(d.getMonth() + 1) + "-" + pad(d.getDate()) + "T" + pad(d.getHours()) + ":" + pad(d.getMinutes()) + ":" + pad(d.getSeconds()) : null;
const ref = o => o ? { id: o.id.primaryKey, name: o.name } : null;
const statusName = s => (/: (\w+)\]/.exec(String(s)) || [])[1];
const match = (o, q) => o.id.primaryKey === q || o.name === q;
const parseDate = s => (s ? new Date(s.length === 10 ? s + "T00:00:00" : s) : null);
const OK = new Set(["Available", "DueSoon", "Next", "Overdue"]);
// Adds the defer/due/planned/completedAt keys that are set.
const withDates = (o, x) => {
  const df = iso(x.deferDate), du = iso(x.dueDate), pl = iso(x.plannedDate), dn = iso(x.completionDate);
  if (df) o.defer = df;
  if (du) o.due = du;
  if (pl) o.planned = pl;
  if (dn) o.completedAt = dn;
  return o;
};
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
  if (t.estimatedMinutes != null) o.estimatedMinutes = t.estimatedMinutes;
  const rr = t.repetitionRule; if (rr) o.repetition = { rule: rr.ruleString, method: statusName(rr.method) };
  return withDates(o, t);
};
// Structure helpers (add/edit/drop/delete). byKind maps a kind to its class; tags nest under tags, the rest under library.
const byKind = { task: Task, project: Project, tag: Tag, folder: Folder };
// Task.byIdentifier(projectId) returns the project's root task; that is a project, not a task.
const isProjectRoot = t => !!(t.containingProject && t.containingProject.task === t);
// Task lookup for every task or parent target; what labels the error ("task", "parent task").
const findTask = (id, what) => {
  const t = Task.byIdentifier(id);
  if (!t) throw new Error((what || "task") + " not found");
  if (isProjectRoot(t)) throw new Error((what || "task") + " " + id + " is a project, use kind project");
  return t;
};
const find = (kind, id) => {
  if (kind === "task") return findTask(id);
  const o = byKind[kind].byIdentifier(id); if (!o) throw new Error(kind + " not found"); return o;
};
// True when x is o or a descendant of o (walks .parent).
const inside = (o, x) => { for (let c = x; c; c = c.parent) if (c.id.primaryKey === o.id.primaryKey) return true; return false; };
// Tasks neither completed nor dropped.
const remaining = ts => ts.filter(t => { const s = statusName(t.taskStatus); return s !== "Completed" && s !== "Dropped"; });
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
