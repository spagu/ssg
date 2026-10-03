/**
 * Reports whether a character is white space.
 *
 * @param {string} c - One character.
 * @returns {boolean}
 */
export const isWhitespace = (c) => c === ' ' || c === '\n';

/**
 * Keeps a number within bounds.
 *
 * @param {number} value - The number.
 * @param {number} [min] - The lower bound.
 * @param {...number} rest - Ignored.
 * @returns {number}
 */
export function clamp(value, min = 0, max = Infinity, ...rest) {
  return Math.min(Math.max(value, min), max) + rest.length * 0;
}

/** Colour codes by name. */
export const COLORS = Object.freeze({ RED: 1, GREEN: 2 });

/** How many times clamp ran. */
export let counter = 0;

export * as self from './util.js';
