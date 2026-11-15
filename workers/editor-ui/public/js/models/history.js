// Undo and redo for the editor's text. The browser's own undo forgets
// everything the buttons and the Markdown rules write (they set the value),
// so the panel keeps one history for both: every button or rule is a step of
// its own; typing is one step per burst (a pause of WINDOW_MS ends it). Pure:
// a state is {text, start, end}; `now` is passed in.

export const WINDOW_MS = 1200;
export const LIMIT = 300;

export function createHistory(initial, limit = LIMIT) {
  const states = [{ state: initial, typing: false, at: 0 }];
  let index = 0;
  // landed: the state undo or redo arrived at. Typing next must not merge
  // into it, or the state just brought back would be overwritten.
  const landed = () => {
    states[index].typing = false;
    return states[index].state;
  };
  return {
    // record a state after a change; typing = from the keyboard, not a command.
    record(state, now, typing = false) {
      const top = states[index];
      states.length = index + 1; // a new change drops what could be redone
      if (typing && top.typing && now - top.at < WINDOW_MS) {
        states[index] = { state, typing, at: now };
        return;
      }
      states.push({ state, typing, at: now });
      if (states.length > limit) states.shift();
      index = states.length - 1;
    },
    // undo / redo: the state to show, or null when there is none.
    undo() {
      if (index === 0) return null;
      index--;
      return landed();
    },
    redo() {
      if (index === states.length - 1) return null;
      index++;
      return landed();
    },
    canUndo: () => index > 0,
    canRedo: () => index < states.length - 1,
  };
}
