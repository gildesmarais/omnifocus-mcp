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
