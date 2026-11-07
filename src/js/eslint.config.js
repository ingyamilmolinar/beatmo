import js from "@eslint/js";
import globals from "globals";

export default [
  {
    ignores: [
      "drums.single.js",
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
  // page.evaluate() runs in the page, where the Go runtime injects globals ESLint cannot see.
  {
    files: ["**/*.browser.test.js", "**/*_helpers.js", "**/*_actions.js"],
    rules: {
      "no-undef": "off",
    },
  },
];
