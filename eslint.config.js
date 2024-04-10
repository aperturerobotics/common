// eslint.config.js
import js from "@eslint/js";

export default [
  js.configs.recommended,
];
// @ts-check

import eslint from '@eslint/js';
import tseslint from 'typescript-eslint';


export default [...tseslint.config(
  eslint.configs.recommended,
  ...tseslint.configs.recommended,
), {
    plugins: ['unused-imports'],
    extends: [
      'eslint:recommended',
      'plugin:@typescript-eslint/recommended',
      'plugin:react-hooks/recommended',
      'prettier',
    ],
    parserOptions: {
      project: './tsconfig.json',
    },
    rules: {
      '@typescript-eslint/explicit-module-boundary-types': 'off',
      '@typescript-eslint/no-non-null-assertion': 'off',
    },
    ignores: ['node_modules', 'dist', 'coverage', 'bundle', 'runtime', 'vendor'],
}];
