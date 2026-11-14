// The panel's two preferences — language and light/dark — remembered per
// browser in localStorage. A convenience only: storage can be missing or
// throw (private windows, blocked site data), and the panel then starts from
// the page's language and "auto".

export const THEMES = ["auto", "light", "dark"];
export const THEME_KEY = "ssg-auth-admin-theme";
export const LANG_KEY = "ssg-auth-admin-lang";

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

// applyTheme sets or clears data-theme on <html> (tokens.css honours it over
// the media query); returns the theme actually shown.
export function applyTheme(root, theme, prefersDark) {
  if (theme === "auto") root.removeAttribute("data-theme");
  else root.setAttribute("data-theme", theme);
  return theme === "auto" ? (prefersDark ? "dark" : "light") : theme;
}
