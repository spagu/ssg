// A typed component call, written from a form (GO-174). The syntax is ssg's
// own (internal/components/call.go): `{{< name key="value" n=3 on=true
// list="a, b" >}}`, on one line, ending at the first `>}}`. Every value this
// module writes is one that parser reads back as typed — the shared samples
// in test/widget-samples.json are parsed by a Go test in that package, so the
// two sides cannot drift apart. A value the syntax cannot carry is refused by
// name, never escaped into something else:
//   oneLine     a line break (a call lives on one line)
//   terminator  the text ">}}" (it would end the call)
//   quotes      both " and ' (a quoted value cannot hold its own quote)
//   number      not a decimal number ssg's strconv reads
//   enum        not one of the listed values
//   required    empty, and the component requires it
//
// Values come from the form as text (string, number, array) or a boolean
// (bool). A value equal to the prop's default is left out: the build fills it.

// NUMBER: what strconv.Atoi or ParseFloat accepts, decimal forms only.
export const NUMBER = /^[+-]?(?:\d+(?:\.\d*)?|\.\d+)(?:[eE][+-]?\d+)?$/;

// defaultText: a prop's default as the form shows it ("" when none).
export function defaultText(prop) {
  if (prop.default === undefined || prop.default === null) return "";
  return Array.isArray(prop.default) ? prop.default.join(", ") : String(prop.default);
}

// initialValues: the form's starting values — the defaults.
export function initialValues(component) {
  return Object.fromEntries(component.props.map((p) => [p.name, p.type === "bool" ? p.default === true : defaultText(p)]));
}

// quoted: the value between quotes the parser reads back exactly, or null.
export function quoted(value) {
  if (!value.includes('"')) return `"${value}"`;
  return value.includes("'") ? null : `'${value}'`;
}

// textProblem: why a value cannot go into a call, or null.
function textProblem(value) {
  if (/[\r\n]/.test(value)) return "oneLine";
  if (value.includes(">}}")) return "terminator";
  return quoted(value) === null ? "quotes" : null;
}

const normalArray = (s) => s.split(",").map((x) => x.trim()).filter(Boolean).join(", ");

// argument: "key=value" for one prop, "" to leave it out, or {error}.
function argument(prop, raw) {
  if (prop.type === "bool") {
    const on = raw === true;
    const def = prop.default === true || prop.default === false ? prop.default : false;
    return prop.required || on !== def ? `${prop.name}=${on}` : "";
  }
  const value = prop.type === "array" ? normalArray(String(raw ?? "")) : prop.type === "number" ? String(raw ?? "").trim() : String(raw ?? "");
  if (value.trim() === "") return prop.required ? { error: "required" } : "";
  if (!prop.required && value === defaultText(prop)) return "";
  if (prop.type === "number") return NUMBER.test(value) ? `${prop.name}=${value}` : { error: "number" };
  if (prop.enum && !prop.enum.includes(value)) return { error: "enum" };
  const problem = textProblem(value);
  return problem ? { error: problem } : `${prop.name}=${quoted(value)}`;
}

// componentCall: {text} — the call — or {errors: {prop: reason}}.
export function componentCall(component, values) {
  const parts = [];
  const errors = {};
  for (const prop of component.props) {
    const arg = argument(prop, values[prop.name]);
    if (typeof arg === "object") errors[prop.name] = arg.error;
    else if (arg) parts.push(arg);
  }
  if (Object.keys(errors).length) return { errors };
  return { text: `{{< ${component.name}${parts.map((p) => " " + p).join("")} >}}` };
}

// callArgs: the key/value pairs of a call's text, read as ssg reads them
// (call.go argRe) — the preview lists them in the widget's frame.
const ARG = /([a-zA-Z_][a-zA-Z0-9_-]*)\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s"']+))/g;
export function callArgs(raw) {
  const inner = /^\{\{<\s*[a-z0-9_-]+((?:\s[^\n]*?)?)\s*>\}\}$/.exec(raw);
  if (!inner) return [];
  return [...inner[1].matchAll(ARG)].map((m) => [m[1], m[2] ?? m[3] ?? m[4]]);
}

// filterWidgets: the items whose name or shown text holds every word of the
// query (any case); shown(item) is the title and description as the panel
// shows them (a block's are in the panel's language).
export function filterWidgets(items, query, shown) {
  const words = String(query).toLowerCase().split(/\s+/).filter(Boolean);
  return items.filter((it) => {
    const hay = `${it.name} ${shown(it)}`.toLowerCase();
    return words.every((w) => hay.includes(w));
  });
}
