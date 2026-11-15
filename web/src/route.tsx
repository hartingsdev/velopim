import { useEffect, useRef, useState, type ReactNode } from "react";

// The address is "#view?key=value&…", so every list with its search and
// filters can be linked, reloaded and reached with the back button.

export const views = ["tasks", "products", "product", "imports", "sync", "sources", "rules", "pricing", "search"] as const;
export type View = (typeof views)[number];
export type Params = Record<string, string | undefined>;

export interface Route {
  view: View;
  params: Params;
}

export function parseRoute(hash: string): Route {
  const [name, query = ""] = hash.replace(/^#/, "").split("?");
  const params: Params = {};
  for (const [key, value] of new URLSearchParams(query)) params[key] = value;
  // Links from before the tabs were merged.
  if (name === "open") return { view: "tasks", params };
  if (name === "closed") return { view: "tasks", params: { ...params, status: "closed" } };
  return { view: (views as readonly string[]).includes(name) ? (name as View) : "tasks", params };
}

export function href(view: View, params: Params = {}): string {
  const q = new URLSearchParams();
  for (const [key, value] of Object.entries(params)) if (value) q.set(key, value);
  const s = q.toString();
  return `#${view}${s ? `?${s}` : ""}`;
}

export function useRoute(): Route {
  const [route, setRoute] = useState(() => parseRoute(window.location.hash));
  useEffect(() => {
    const onHash = () => setRoute(parseRoute(window.location.hash));
    window.addEventListener("hashchange", onHash);
    return () => window.removeEventListener("hashchange", onHash);
  }, []);
  return route;
}

/** A filter shown as a pill; it is a link, so it works with the back button. */
export function Chip(props: { view: View; params: Params; active: boolean; children: ReactNode }) {
  return (
    <a href={href(props.view, props.params)} className={props.active ? "chip active" : "chip"} aria-current={props.active}>
      {props.children}
    </a>
  );
}

/** Search field of a list. Typing updates the address after a short pause. */
export function SearchBox(props: { view: View; params: Params; placeholder: string }) {
  const { view, params, placeholder } = props;
  const [text, setText] = useState(params.q ?? "");
  const timer = useRef<number | undefined>(undefined);

  // Follow the address when it changes from outside (back button, links).
  useEffect(() => setText(params.q ?? ""), [params.q]);
  useEffect(() => () => window.clearTimeout(timer.current), []);

  function change(value: string) {
    setText(value);
    window.clearTimeout(timer.current);
    timer.current = window.setTimeout(() => {
      // Replace instead of push: one history entry per search, not per letter.
      window.location.replace(href(view, { ...params, q: value.trim() || undefined }));
    }, 250);
  }

  return (
    <input
      type="search"
      className="search"
      value={text}
      onChange={(e) => change(e.target.value)}
      placeholder={placeholder}
      aria-label={placeholder}
    />
  );
}

/** True when every word of the query occurs in the text; for small lists filtered in the browser. */
export function matches(query: string | undefined, ...texts: (string | number | null | undefined)[]): boolean {
  if (!query) return true;
  const haystack = texts.filter((t) => t != null).join(" ").toLowerCase();
  return query.toLowerCase().split(/\s+/).every((word) => haystack.includes(word));
}
