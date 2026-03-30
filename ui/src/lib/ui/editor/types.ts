export type EditorLanguage = 'cel' | 'hl7v2' | 'yaml' | 'json' | 'text';

export interface EditorDiagnostic {
  from: number;
  to: number;
  severity: 'error' | 'warning' | 'info' | 'hint';
  message: string;
}

export interface CodeEditorProps {
  language: EditorLanguage;
  value: string;
  readOnly?: boolean;
  lineNumbers?: boolean;
  diagnostics?: EditorDiagnostic[];
  placeholder?: string;
  height?: string;
}
