// The two pieces of the chrome that need scripting: the colour-scheme toggle
// and the current-page marker in the navigation. Both are enhancements — the
// page is correct without either.

const KEY = "picostore-scheme";

function currentScheme() {
  if (document.documentElement.dataset.theme) return document.documentElement.dataset.theme;
  return window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
}

function bindSchemeToggle() {
  const button = document.getElementById("ps-scheme");
  if (!button) return;
  const describe = () => {
    const next = currentScheme() === "dark" ? "light" : "dark";
    button.setAttribute("aria-label", `Switch to the ${next} colour scheme`);
  };
  describe();
  button.addEventListener("click", () => {
    const next = currentScheme() === "dark" ? "light" : "dark";
    document.documentElement.dataset.theme = next;
    try {
      localStorage.setItem(KEY, next);
    } catch (e) {
      // Private window or blocked storage: the choice holds for this page only.
      console.info("picostore: colour scheme not saved", e.name);
    }
    describe();
  });
}

/** Marks the navigation link for the page you are on. Done here rather than in
 *  the template because the same header is rendered into every page. */
function markCurrentPage() {
  const here = window.location.pathname.replace(/\/+$/, "") || "/";
  for (const link of document.querySelectorAll(".ps-nav a[href]")) {
    const target = new URL(link.href, window.location.origin).pathname.replace(/\/+$/, "") || "/";
    if (target === here) link.setAttribute("aria-current", "page");
  }
}

document.addEventListener("DOMContentLoaded", () => {
  bindSchemeToggle();
  markCurrentPage();
});
