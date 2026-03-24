/**
 * HL7v2 language mode for CodeMirror 6.
 *
 * Provides syntax highlighting for HL7v2 messages with segment headers,
 * field separators, component separators, and other delimiters.
 */
import { StreamLanguage, type StreamParser } from '@codemirror/language';

const SEGMENT_HEADER = /^[A-Z][A-Z0-9]{2}/;

interface HL7State {
  lineStart: boolean;
}

const hl7v2Parser: StreamParser<HL7State> = {
  startState(): HL7State {
    return { lineStart: true };
  },

  token(stream, state): string | null {
    // At beginning of line, check for segment header
    if (state.lineStart) {
      state.lineStart = false;
      if (stream.match(SEGMENT_HEADER)) {
        return 'keyword';
      }
    }

    // Track newlines
    if (stream.eol()) {
      state.lineStart = true;
    }

    const ch = stream.next();
    if (ch === null) return null;

    switch (ch) {
      case '|':
        return 'operator';
      case '^':
        return 'punctuation';
      case '~':
        return 'meta';
      case '\\':
        // Escape character - consume the next character too
        stream.next();
        return 'escape';
      case '&':
        return 'punctuation';
      default:
        // Consume regular text until a delimiter
        stream.eatWhile(/[^|^~\\&\n]/);
        return 'string';
    }
  }
};

export function hl7v2() {
  return StreamLanguage.define(hl7v2Parser);
}
