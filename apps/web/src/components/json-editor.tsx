import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import { Compartment, EditorState } from '@codemirror/state'
import { EditorView, keymap, lineNumbers, drawSelection } from '@codemirror/view'
import { defaultKeymap, history, historyKeymap } from '@codemirror/commands'
import { json } from '@codemirror/lang-json'
import { bracketMatching, defaultHighlightStyle, syntaxHighlighting } from '@codemirror/language'

export interface JsonEditorProps {
  value: string
  onChange?: (value: string) => void
  readOnly?: boolean
  label?: string
}

/** Document and undo history live only in this mounted editor. Tab keeps its normal focus behavior. */
export function JsonEditor({ value, onChange, readOnly = false, label = 'JSON' }: JsonEditorProps) {
  const host = useRef<HTMLDivElement>(null)
  const view = useRef<EditorView | null>(null)
  const listener = useRef(onChange)
  const [initial] = useState(value)
  const [access] = useState(() => new Compartment())
  useEffect(() => {
    listener.current = onChange
  }, [onChange])
  useLayoutEffect(() => {
    const editor = new EditorView({
      parent: host.current!,
      state: EditorState.create({
        doc: initial,
        extensions: [
          json(),
          lineNumbers(),
          drawSelection(),
          history(),
          bracketMatching(),
          syntaxHighlighting(defaultHighlightStyle),
          keymap.of([...defaultKeymap, ...historyKeymap]),
          access.of([]),
          EditorView.updateListener.of((update) => {
            if (update.docChanged) listener.current?.(update.state.doc.toString())
          }),
          EditorView.theme({
            '&': {
              backgroundColor: 'var(--background)',
              color: 'var(--foreground)',
              fontSize: '16px',
              height: 'var(--json-editor-height, min(45dvh, 24rem))',
            },
            '.cm-scroller': {
              fontFamily: 'ui-monospace, monospace',
              overflow: 'auto',
              overscrollBehavior: 'contain',
            },
            '.cm-gutters': {
              backgroundColor: 'var(--muted)',
              color: 'var(--muted-foreground)',
              border: 'none',
            },
            '.cm-content': { padding: '8px 0', minHeight: '100%', caretColor: 'var(--foreground)' },
            '&.cm-focused': { outline: '2px solid var(--ring)', outlineOffset: '-2px' },
            '.cm-selectionBackground, &.cm-focused .cm-selectionBackground': {
              backgroundColor: 'color-mix(in srgb, var(--primary) 20%, transparent)',
            },
          }),
        ],
      }),
    })
    view.current = editor
    return () => {
      view.current = null
      editor.destroy()
    }
  }, [access, initial])
  useLayoutEffect(() => {
    view.current?.dispatch({
      effects: access.reconfigure([
        EditorState.readOnly.of(readOnly),
        EditorView.editable.of(!readOnly),
        EditorView.contentAttributes.of({
          'aria-label': label,
          spellcheck: 'false',
          autocapitalize: 'off',
          autocomplete: 'off',
        }),
      ]),
    })
  }, [access, readOnly, label])
  useLayoutEffect(() => {
    const editor = view.current
    if (editor && editor.state.doc.toString() !== value)
      editor.dispatch({ changes: { from: 0, to: editor.state.doc.length, insert: value } })
  }, [value])
  return (
    <div
      ref={host}
      className="min-w-0 overflow-hidden rounded-md border"
      data-testid="json-editor"
    />
  )
}
