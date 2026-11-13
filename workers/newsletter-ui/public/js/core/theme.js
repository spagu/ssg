// Light, dark, or the system's choice. "auto" follows prefers-color-scheme in
// CSS; "light" / "dark" set data-theme on <html>, which the stylesheet honours
// over the media query. The choice is remembered per browser in localStorage —
// a convenience only: storage can be missing or throw (private windows,
// blocked site data), and the panel then simply starts on "auto".

export const THEMES = ["auto", "light", "dark"];
export const STORAGE_KEY = "newsletter-admin-theme";

export function readTheme(storage) {
  try {
    const v = storage && storage.getItem(STORAGE_KEY);
    return THEMES.includes(v) ? v : "auto";
  } catch {
    return "auto";
  }
}

export function writeTheme(storage, theme) {
  try {
    if (theme === "auto") storage.removeItem(STORAGE_KEY);
    else storage.setItem(STORAGE_KEY, theme);
    return true;
  } catch {
    return false;
  }
}

// applyTheme sets or clears data-theme; returns the theme actually shown.
export function applyTheme(root, theme, prefersDark) {
  if (theme === "auto") root.removeAttribute("data-theme");
  else root.setAttribute("data-theme", theme);
  return theme === "auto" ? (prefersDark ? "dark" : "light") : theme;
}

// The same storage, for the language choice.
export const LANG_KEY = "newsletter-admin-lang";

export function readStored(storage, key) {
  try {
    return (storage && storage.getItem(key)) || null;
  } catch {
    return null;
  }
}

export function writeStored(storage, key, value) {
  try {
    storage.setItem(key, value);
    return true;
  } catch {
    return false;
  }
}
