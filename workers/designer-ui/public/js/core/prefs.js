// The panel's preferences in this browser: light, dark or the system's
// choice, and the language. "auto" follows prefers-color-scheme in CSS;
// "light" / "dark" set data-theme on <html>, which the stylesheet honours over
// the media query. Remembered in localStorage — a convenience only: storage
// can be missing or throw (private windows, blocked site data), and the panel
// then starts on "auto" and the browser's language. Generic: the keys carry a
// prefix the panel chooses, so two panels on one host keep their own.

export const THEMES = ["auto", "light", "dark"];

export function createPrefs(storage, prefix) {
  const read = (key) => {
    try {
      return (storage && storage.getItem(`${prefix}-${key}`)) || null;
    } catch {
      return null;
    }
  };
  const write = (key, value) => {
    try {
      if (value === null) storage.removeItem(`${prefix}-${key}`);
      else storage.setItem(`${prefix}-${key}`, value);
      return true;
    } catch {
      return false;
    }
  };
  return {
    read,
    write,
    theme: () => (THEMES.includes(read("theme")) ? read("theme") : "auto"),
    setTheme: (theme) => write("theme", theme === "auto" ? null : theme),
  };
}

// applyTheme sets or clears data-theme; returns the theme actually shown.
export function applyTheme(root, theme, prefersDark) {
  if (theme === "auto") root.removeAttribute("data-theme");
  else root.setAttribute("data-theme", theme);
  return theme === "auto" ? (prefersDark ? "dark" : "light") : theme;
}
