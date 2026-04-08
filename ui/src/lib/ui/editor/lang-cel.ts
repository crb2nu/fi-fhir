/**
 * CEL (Common Expression Language) mode for CodeMirror 6.
 *
 * Provides syntax highlighting for CEL expressions used in workflow filters.
 */
import { StreamLanguage, type StreamParser } from '@codemirror/language';
import { hoverTooltip } from '@codemirror/view';
import { celAutocomplete } from './celAutocomplete';

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

const celHover = hoverTooltip((view, pos, side) => {
  const { from, to, text } = view.state.doc.lineAt(pos);
  let start = pos, end = pos;
  while (start > from && /\w/.test(text[start - 1 - from]!)) start--;
  while (end < to && /\w/.test(text[end - from]!)) end++;
  if (start === pos && side < 0 || start === end) return null;
  const word = text.slice(start - from, end - from);
  
  const docs: Record<string, string> = {
    'has': 'Checks if a field is present in the message.',
    'size': 'Returns the number of elements in a list, characters in a string, or entries in a map.',
    'matches': 'Checks if the string matches the given regular expression.',
    'type': 'Returns the type name of the value.',
    'timestamp': 'Converts a string or int to a timestamp.',
    'duration': 'Converts a string to a duration.',
    'event': 'The semantic event being processed. Contains properties like `type`, `id`, and `source`.',
  };

  if (Object.hasOwn(docs, word)) {
    return {
      pos: start,
      end,
      above: true,
      create() {
        const dom = document.createElement("div");
        dom.textContent = docs[word]!;
        dom.style.padding = "4px 8px";
        dom.style.fontFamily = "var(--font-sans)";
        dom.style.fontSize = "0.85rem";
        dom.style.color = "var(--color-text-secondary)";
        dom.style.maxWidth = "300px";
        dom.style.lineHeight = "1.4";
        return { dom };
      }
    };
  }
  return null;
});

export function cel() {
  return [StreamLanguage.define(celParser), celAutocomplete(), celHover];
}
