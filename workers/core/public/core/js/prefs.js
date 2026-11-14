// Light, dark, or the system's choice, and the language — remembered per
// browser in localStorage. A convenience only: storage can be missing or
// throw (private windows, blocked site data), and the page then simply starts
// on "auto" and the browser's language. "auto" follows prefers-color-scheme
// in CSS; "light" / "dark" set data-theme on <html>, which the tokens honour
// over the media query.

export const THEMES = ["auto", "light", "dark"];
export const THEME_KEY = "ssg-core-theme";
export const LANG_KEY = "ssg-core-lang";

export function readStored(storage, key) {
  try {
    return (storage && storage.getItem(key)) || null;
  } catch {
    return null;
  }
}

export function writeStored(storage, key, value) {
  try {
    if (value === null) storage.removeItem(key);
    else storage.setItem(key, value);
    return true;
  } catch {
    return false;
  }
}

export const readTheme = (storage) => {
  const v = readStored(storage, THEME_KEY);
  return THEMES.includes(v) ? v : "auto";
};
export const writeTheme = (storage, theme) => writeStored(storage, THEME_KEY, theme === "auto" ? null : theme);

// applyTheme sets or clears data-theme; returns the theme actually shown.
export function applyTheme(root, theme, prefersDark) {
  if (theme === "auto") root.removeAttribute("data-theme");
  else root.setAttribute("data-theme", theme);
  return theme === "auto" ? (prefersDark ? "dark" : "light") : theme;
}
