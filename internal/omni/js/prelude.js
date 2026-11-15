// Prepended by the bridge to every script. Shared helpers only; no side effects.
const pad = n => (n < 10 ? "0" : "") + n;
const iso = d => d ? d.getFullYear() + "-" + pad(d.getMonth() + 1) + "-" + pad(d.getDate()) + "T" + pad(d.getHours()) + ":" + pad(d.getMinutes()) + ":" + pad(d.getSeconds()) : null;
const ref = o => o ? { id: o.id.primaryKey, name: o.name } : null;
const statusName = s => (/: (\w+)\]/.exec(String(s)) || [])[1];
const match = (o, q) => o.id.primaryKey === q || o.name === q;
