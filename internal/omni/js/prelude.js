// Prepended by the bridge to every script. Shared helpers only; no side effects.
const pad = n => (n < 10 ? "0" : "") + n;
const iso = d => d ? d.getFullYear() + "-" + pad(d.getMonth() + 1) + "-" + pad(d.getDate()) + "T" + pad(d.getHours()) + ":" + pad(d.getMinutes()) + ":" + pad(d.getSeconds()) : null;
const ref = o => o ? { id: o.id.primaryKey, name: o.name } : null;
const statusName = s => (/: (\w+)\]/.exec(String(s)) || [])[1];
const match = (o, q) => o.id.primaryKey === q || o.name === q;
const parseDate = s => (s ? new Date(s.length === 10 ? s + "T00:00:00" : s) : null);
// Range filter on a date field, the one owner of how bounds match. Go has resolved args[k + "_after"] and args[k + "_before"] to full timestamps (resolveBound in read.go expands date-only values). Returns a test for a Date or null, or null when neither bound is set. Both bounds are inclusive and a missing date never matches. Dates are reported to the second, so a _before bound includes the whole second it names (a date's fraction would otherwise push it out of its own bound).
const dateBounds = (args, k) => {
  const a = args[k + "_after"], b = args[k + "_before"];
  const lo = a ? new Date(a) : null, hi = b ? new Date(new Date(b).getTime() + 1000) : null;
  return lo || hi ? d => !!d && (!lo || d >= lo) && (!hi || d < hi) : null;
};
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
// A repetition rule as Repetition in read.go (enums print as "[object ...: Name]").
const shapeRepetition = r => ({ rule: r.ruleString, scheduleType: statusName(r.scheduleType), anchor: statusName(r.anchorDateKey), catchUp: r.catchUpAutomatically });
// Adds estimatedMinutes and repetition when set.
const withEstimateRepetition = (o, x) => {
  if (x.estimatedMinutes != null) o.estimatedMinutes = x.estimatedMinutes;
  const rr = x.repetitionRule; if (rr) o.repetition = shapeRepetition(rr);
  return o;
};
// A Repetition from Go as a rule. Go has validated the names against the allowed sets; the guard stops a name that drifts from OmniFocus's enums.
const makeRule = r => {
  const st = Task.RepetitionScheduleType[r.scheduleType], an = Task.AnchorDateKey[r.anchor];
  if (!st || !an) throw new Error("repetition: unknown scheduleType or anchor");
  return new Task.RepetitionRule(r.rule, null, st, an, r.catchUp);
};
// A task as the Task record in read.go.
const shapeTask = t => {
  const st = statusName(t.taskStatus);
  const o = {
    id: t.id.primaryKey, name: t.name, status: st, flagged: t.flagged,
    completed: t.completed, dropped: st === "Dropped", available: OK.has(st), blocked: st === "Blocked",
    inbox: t.inInbox, tags: t.tags.map(ref),
    hasChildren: t.hasChildren, sequential: t.sequential, completedByChildren: t.completedByChildren,
    added: iso(t.added), modified: iso(t.modified)
  };
  if (t.note) o.note = t.note;
  const p = ref(t.containingProject); if (p) o.project = p;
  const par = ref(t.parent); if (par) o.parent = par;
  const dd = iso(t.dropDate); if (dd) o.dropDate = dd;
  return withEstimateRepetition(withDates(o, t), t);
};
// A project's type from its two flags; OmniFocus allows both at once, and a single-actions list wins.
const projectType = p => p.containsSingletonActions ? "SingleActions" : p.sequential ? "Sequential" : "Parallel";
// Writes both flags so the type reads back whatever they were.
const setProjectType = (p, type) => { p.sequential = type === "Sequential"; p.containsSingletonActions = type === "SingleActions"; };
const has = (p, k) => Object.prototype.hasOwnProperty.call(p, k);
// Throws "<what> not found" for a missing lookup result, else returns it.
const need = (o, what) => { if (!o) throw new Error(what + " not found"); return o; };
const tagsOf = ids => (ids || []).map(id => need(Tag.byIdentifier(id), "tag " + id));
// Validates the project fields of an add or edit item and looks up its tags before any mutation, then returns the function applying them.
// Absent keys are untouched and null clears, so add (keys only when set) and edit share it; tagsAdd/tagsRemove are tag ids.
const projectWriter = (p, tagsAdd, tagsRemove) => {
  const add = tagsOf(tagsAdd), rem = tagsOf(tagsRemove);
  const rule = has(p, "repetition") && p.repetition !== null ? makeRule(p.repetition) : null;
  const date = (k, set) => { if (has(p, k)) set(p[k] === null ? null : parseDate(p[k])); };
  return o => {
    if (p.type) setProjectType(o, p.type);
    if (has(p, "note")) o.note = p.note === null ? "" : p.note;
    if (has(p, "flagged")) o.flagged = p.flagged;
    date("defer", d => { o.deferDate = d; });
    date("due", d => { o.dueDate = d; });
    date("planned", d => { o.plannedDate = d; });
    if (has(p, "estimatedMinutes")) o.estimatedMinutes = p.estimatedMinutes;
    if (has(p, "repetition")) o.repetitionRule = rule;
    if (has(p, "completedByChildren")) o.completedByChildren = p.completedByChildren;
    add.forEach(g => o.addTag(g));
    rem.forEach(g => o.removeTag(g));
  };
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
