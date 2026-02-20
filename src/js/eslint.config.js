import js from "@eslint/js";
import globals from "globals";

export default [
  {
    ignores: [
      "drums.single.js",
      "drums.js",
      "audio_config.generated.js",
      "wasm_exec.js",
      "node_modules/",
      "recordings/",
      "testdata/",
      "llm_test/",
      "*.wasm",
    ],
  },
  js.configs.recommended,
  {
    languageOptions: {
      ecmaVersion: 2022,
      sourceType: "module",
      globals: {
        ...globals.browser,
        ...globals.node,
        // Go WASM runtime globals
        Go: "readonly",
      },
    },
    rules: {
      "no-unused-vars": ["warn", {
        argsIgnorePattern: "^_",
        varsIgnorePattern: "^_",
        caughtErrorsIgnorePattern: "^_",
      }],
      "no-constant-condition": ["error", { checkLoops: false }],
      "no-empty": ["error", { allowEmptyCatch: true }],
    },
  },
  // Browser test files and helpers: disable no-undef because page.evaluate()
  // runs code in the browser WASM context where globals like startPlay,
  // stopPlay, forceDraw, etc. are injected by the Go WASM runtime.
  // ESLint cannot reason about cross-context evaluate() scope.
  {
    files: ["**/*.browser.test.js", "**/*_helpers.js", "**/*_actions.js"],
    rules: {
      "no-undef": "off",
    },
  },
];
