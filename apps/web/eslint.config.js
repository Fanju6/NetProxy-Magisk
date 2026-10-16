import js from '@eslint/js'
import globals from 'globals'
import tseslint from 'typescript-eslint'
import hooks from 'eslint-plugin-react-hooks'
import refresh from 'eslint-plugin-react-refresh'
import prettier from 'eslint-config-prettier'

export default tseslint.config(
  {
    ignores: [
      'dist',
      'dist-native',
      'node_modules',
      'playwright-report',
      'test-results',
      'pnpm-lock.yaml',
    ],
  },
  js.configs.recommended,
  tseslint.configs.recommended,
  { files: ['**/*.{js,mjs}'], languageOptions: { globals: globals.node } },
  {
    files: ['**/*.{ts,tsx}'],
    languageOptions: { globals: { ...globals.browser, ...globals.node } },
  },
  {
    files: ['src/**/*.{ts,tsx}'],
    plugins: { 'react-hooks': hooks, 'react-refresh': refresh },
    rules: {
      ...hooks.configs.recommended.rules,
      'react-refresh/only-export-components': ['error', { allowConstantExport: true }],
    },
  },
  // 官方组件导出变体、上下文和 Hook，不对上游生成文件施加页面组件的 Fast Refresh 限制。
  {
    files: ['src/components/ui/**', 'src/api/events.tsx'],
    rules: { 'react-refresh/only-export-components': 'off' },
  },
  prettier,
)
