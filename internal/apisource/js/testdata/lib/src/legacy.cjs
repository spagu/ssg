'use strict';

/**
 * Parses the old format.
 *
 * @param {string} text - The input.
 * @returns {string[]}
 */
function legacyParse(text) {
  return text.split(',');
}

module.exports = {
  legacyParse,
  /** The format version. */
  VERSION: '0.1',
  /**
   * Helps.
   *
   * @param {number} n
   */
  helper(n) {
    return n;
  },
};

/** An extra export. */
exports.extra = function () {};
