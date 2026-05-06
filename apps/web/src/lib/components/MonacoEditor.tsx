import { useEffect, useRef } from 'react';
import type * as Monaco from 'monaco-editor/esm/vs/editor/editor.api';

type MonacoApi = typeof import('monaco-editor/esm/vs/editor/editor.api');

let monacoApiPromise: Promise<MonacoApi> | null = null;
const loadedLanguages = new Set<string>();

function loadMonacoApi() {
  monacoApiPromise ??= import('monaco-editor/esm/vs/editor/editor.api');
  return monacoApiPromise;
}

function resolveMonacoLanguage(input: string) {
  if (input === 'text') return 'plaintext';
  if (input === 'toml') return 'ini';
  return input;
}

async function loadMonacoLanguage(input: string) {
  const language = resolveMonacoLanguage(input);
  if (loadedLanguages.has(language)) return language;

  switch (language) {
    case 'json':
      await import('monaco-editor/esm/vs/language/json/monaco.contribution');
      break;
    case 'javascript':
    case 'typescript':
      await import('monaco-editor/esm/vs/language/typescript/monaco.contribution');
      break;
    case 'markdown':
      await import('monaco-editor/esm/vs/basic-languages/markdown/markdown.contribution');
      break;
    case 'python':
      await import('monaco-editor/esm/vs/basic-languages/python/python.contribution');
      break;
    case 'sql':
      await import('monaco-editor/esm/vs/basic-languages/sql/sql.contribution');
      break;
    case 'r':
      await import('monaco-editor/esm/vs/basic-languages/r/r.contribution');
      break;
    case 'ini':
      await import('monaco-editor/esm/vs/basic-languages/ini/ini.contribution');
      break;
  }

  loadedLanguages.add(language);
  return language;
}

interface MonacoEditorProps {
  value: string;
  language: string;
  minHeight?: number;
  onChange?: (value: string) => void;
  onBlur?: (value: string) => void;
}

export function MonacoEditor({ value, language, minHeight = 160, onChange, onBlur }: MonacoEditorProps) {
  const containerRef = useRef<HTMLDivElement | null>(null);
  const editorRef = useRef<Monaco.editor.IStandaloneCodeEditor | null>(null);
  const monacoRef = useRef<MonacoApi | null>(null);
  const syncingRef = useRef(false);
  const callbacksRef = useRef({ onChange, onBlur });

  // Mirror the latest callbacks without re-running the init effect.
  useEffect(() => {
    callbacksRef.current = { onChange, onBlur };
  }, [onChange, onBlur]);

  // Init + dispose lifecycle. Captures `value` and `language` only as initial
  // seed; later changes flow through the sync effects below.
  useEffect(() => {
    let disposed = false;
    let changeSubscription: Monaco.IDisposable | null = null;
    let blurSubscription: Monaco.IDisposable | null = null;

    (async () => {
      const [monaco, resolvedLanguage] = await Promise.all([
        loadMonacoApi(),
        loadMonacoLanguage(language),
      ]);

      if (disposed || !containerRef.current) return;

      monacoRef.current = monaco;
      const editor = monaco.editor.create(containerRef.current, {
        value,
        language: resolvedLanguage,
        automaticLayout: true,
        minimap: { enabled: false },
        fontSize: 13,
        lineNumbers: 'on',
        roundedSelection: false,
        scrollBeyondLastLine: false,
        wordWrap: resolvedLanguage === 'markdown' ? 'on' : 'off',
        theme: 'vs',
      });

      if (disposed) {
        editor.dispose();
        return;
      }

      editorRef.current = editor;
      changeSubscription = editor.onDidChangeModelContent(() => {
        if (syncingRef.current) return;
        callbacksRef.current.onChange?.(editor.getValue());
      });
      blurSubscription = editor.onDidBlurEditorText(() => {
        callbacksRef.current.onBlur?.(editor.getValue());
      });
    })();

    return () => {
      disposed = true;
      changeSubscription?.dispose();
      blurSubscription?.dispose();
      editorRef.current?.dispose();
      editorRef.current = null;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Sync external value changes into the editor without echoing back through onChange.
  useEffect(() => {
    const editor = editorRef.current;
    if (!editor || editor.getValue() === value) return;
    syncingRef.current = true;
    editor.setValue(value);
    syncingRef.current = false;
  }, [value]);

  // Switch language at runtime without recreating the editor.
  useEffect(() => {
    const editor = editorRef.current;
    const monaco = monacoRef.current;
    if (!editor || !monaco) return;
    const model = editor.getModel();
    if (!model) return;

    let canceled = false;
    (async () => {
      const resolved = await loadMonacoLanguage(language);
      if (canceled) return;
      monaco.editor.setModelLanguage(model, resolved);
      editor.updateOptions({ wordWrap: resolved === 'markdown' ? 'on' : 'off' });
    })();
    return () => {
      canceled = true;
    };
  }, [language]);

  return (
    <div ref={containerRef} style={{ width: '100%', height: Math.max(minHeight, 96) }} />
  );
}
