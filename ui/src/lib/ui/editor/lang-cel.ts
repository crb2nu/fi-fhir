/**
 * CEL (Common Expression Language) mode for CodeMirror 6.
 *
 * Provides syntax highlighting for CEL expressions used in workflow filters.
 */
import { StreamLanguage, type StreamParser } from '@codemirror/language';

const CEL_KEYWORDS = new Set([
  'true', 'false', 'null', 'in', 'has', 'size', 'matches',
  'exists', 'all', 'map', 'filter', 'exists_one', 'type',
  'duration', 'timestamp'
]);

const CEL_OPERATORS = /^(&&|\|\||!=|==|<=|>=|[+\-*/%<>!?:.])/;

interface CELState {
  inString: false | '"' | "'";
}

const celParser: StreamParser<CELState> = {
  startState(): CELState {
    return { inString: false };
  },

  token(stream, state): string | null {
    // Continue string
    if (state.inString) {
      const quote = state.inString;
      while (!stream.eol()) {
        const ch = stream.next();
        if (ch === '\\') {
          stream.next(); // skip escaped char
        } else if (ch === quote) {
          state.inString = false;
          return 'string';
        }
      }
      return 'string';
    }

    // Skip whitespace
    if (stream.eatSpace()) return null;

    // Line comment
    if (stream.match('//')) {
      stream.skipToEnd();
      return 'comment';
    }

    // String literals
    const ch = stream.peek();
    if (ch === '"' || ch === "'") {
      state.inString = ch as '"' | "'";
      stream.next();
      return 'string';
    }

    // Numbers
    if (stream.match(/^[0-9]+(\.[0-9]+)?([eE][+-]?[0-9]+)?/)) {
      return 'number';
    }

    // Operators
    if (stream.match(CEL_OPERATORS)) {
      return 'operator';
    }

    // Brackets/parens
    if (stream.match(/^[()[\]{},]/)) {
      return 'bracket';
    }

    // Keywords and identifiers
    if (stream.match(/^[a-zA-Z_][a-zA-Z0-9_]*/)) {
      const word = stream.current();
      if (CEL_KEYWORDS.has(word)) {
        return 'keyword';
      }
      return 'variableName';
    }

    // Advance past unknown character
    stream.next();
    return null;
  }
};

export function cel() {
  return StreamLanguage.define(celParser);
}
